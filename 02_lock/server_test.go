package lock_test

import (
	"fmt"
	"io"
	"lazydb/lock"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"golang.org/x/sync/errgroup"
)

func TestServer(t *testing.T) {
	s := lock.NewServer()
	testServer(s, t)
}

func testServer(s *lock.Server, t testing.TB) {
	ts := httptest.NewServer(http.HandlerFunc(s.GetMyData))
	defer ts.Close()

	tenantID := "foo"
	testRequest(ts.URL, tenantID, t)
}

func testRequest(endpoint string, tenantID string, t testing.TB) {
	url := fmt.Sprintf("%s?tenant=%s", endpoint, tenantID)
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	result, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}

	if want, got := `value: "value_for_mykey"`, string(result); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func init() {
	// see https://github.com/golang/go/issues/16012#issuecomment-224948823
	http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = 100
}

func BenchmarkServer(b *testing.B) {
	nbTenants := 4
	maxConcurrentRequests := 60
	nbRequests := 300

	b.ResetTimer()
	totalcrea := uint64(0)
	for i := 0; i < b.N; i++ {
		s := lock.NewServer()
		ts := httptest.NewServer(http.HandlerFunc(s.GetMyData))

		g := new(errgroup.Group)
		g.SetLimit(maxConcurrentRequests)
		for j := 0; j < nbRequests; j++ {
			tenantID := "foo" + strconv.Itoa((j*2417)%nbTenants)
			g.Go(func() error {
				testRequest(ts.URL, tenantID, b)
				return nil
			})
		}
		err := g.Wait()
		if err != nil {
			b.Error(err)
		}

		ts.Close()
		totalcrea += s.Count()
	}
	b.Logf("benchmark created %d connections in %d iterations, which is on average %d", totalcrea, b.N, (totalcrea / uint64(b.N)))
}
