package web

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"

	credmodels "bundle-credit-test/internal/credits/models"
	custmodels "bundle-credit-test/internal/customer/models"
	"bundle-credit-test/internal/platform/ids"
)

func (s *Server) chargeReq(c *fiber.Ctx, me custmodels.Customer, key, input string) credmodels.ChargeRequest {
	return credmodels.ChargeRequest{CustomerID: me.ID, AppID: c.FormValue("app"), Action: c.FormValue("action"), Key: key, Input: input}
}

func (s *Server) simCharge(ctx context.Context, c *fiber.Ctx, me custmodels.Customer) error {
	r := s.chargeReq(c, me, ids.New("key_"), "photo.jpg")
	code, res := s.a.Credits.Charge(ctx, r)
	note := res.Error
	if res.Blocked {
		note = "content blocked by policy — still charged, the provider did the work"
	}
	s.a.Hub.Log(logKind(code), "%s → %s: %d %s %s (available here: %d)", me.Handle, r.Action, code, res.Status, note, res.Available)
	return nil
}

// simRace fires n separate actions at once from n devices and checks the balance moved by exactly the successes.
func (s *Server) simRace(ctx context.Context, c *fiber.Ctx, me custmodels.Customer) error {
	n, _ := strconv.Atoi(c.FormValue("n"))
	if n <= 0 {
		n = 5
	}
	app, action := c.FormValue("app"), c.FormValue("action")
	before := s.a.Credits.Available(ctx, me.ID, app)
	s.a.Hub.Log("info", "race: %d devices of %s fire %s at the same time — %d separate actions, different keys; available here %d", n, me.Handle, action, n, before)
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var res credmodels.ChargeResult
			codes[i], res = s.a.Credits.Charge(ctx, s.chargeReq(c, me, ids.New("race_"), fmt.Sprintf("device-%d", i)))
			if codes[i] >= 500 {
				s.a.Hub.Log("err", "device-%d: %d %s", i, codes[i], res.Error)
			}
		}(i)
	}
	wg.Wait()
	after := s.a.Credits.Available(ctx, me.ID, app)
	ok := count(codes, 201)
	price, _ := s.a.Catalog.ActionPrice(ctx, app, action, s.a.Clock.Now(ctx))
	verdict := "consistent"
	if before-after != int64(ok)*price {
		verdict = "MISMATCH"
	}
	s.a.Hub.Log("info", "race result: %s · balance %d → %d (−%d = %d successes × %d cr, %s)", summarize(codes), before, after, before-after, ok, price, verdict)
	if ok == n {
		s.a.Hub.Log("info", "enough credits for every action, so all succeed. To see 402s, race on a small balance: bob → Credit pack 500 + App Shop monthly, then Race ×5 in App Shop twice")
	}
	return nil
}

// simRetry sends one action three times at once with the same key, then again, then with another body.
func (s *Server) simRetry(ctx context.Context, c *fiber.Ctx, me custmodels.Customer) error {
	key := ids.New("retry_")
	s.a.Hub.Log("info", "retry: same key %s sent 3× at once (two Gateway instances + client retry)", key)
	codes := make([]int, 3)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], _ = s.a.Credits.Charge(ctx, s.chargeReq(c, me, key, "photo.jpg"))
		}(i)
	}
	wg.Wait()
	s.a.Hub.Log("info", "concurrent copies: %s", summarize(codes))
	code, res := s.a.Credits.Charge(ctx, s.chargeReq(c, me, key, "photo.jpg"))
	s.a.Hub.Log(logKind(code), "retry after the first one finished → %d %s (replayed from the stored charge, nothing charged again)", code, res.Status)
	code, res = s.a.Credits.Charge(ctx, s.chargeReq(c, me, key, "other.jpg"))
	s.a.Hub.Log(logKind(code), "same key, different body → %d %s", code, res.Error)
	return nil
}

func summarize(codes []int) string {
	m := map[int]int{}
	for _, c := range codes {
		m[c]++
	}
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d×%d", m[k], k))
	}
	return strings.Join(parts, ", ")
}

func count(codes []int, want int) int {
	n := 0
	for _, c := range codes {
		if c == want {
			n++
		}
	}
	return n
}
