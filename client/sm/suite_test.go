package sm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type FakeAuthClient struct {
	AccessToken string
	requestURI  string
}

func (c *FakeAuthClient) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	c.requestURI = req.URL.RequestURI()
	return http.DefaultClient.Do(req)
}

type HandlerDetails struct {
	Method             string
	Path               string
	ResponseBody       []byte
	ResponseStatusCode int
	Headers            map[string]string
}

func TestSMClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "")
}

var (
	client         Client
	handlerDetails []HandlerDetails
	smServer       *httptest.Server
	fakeAuthClient *FakeAuthClient

	validToken = "valid-token"
	params     = &Parameters{
		GeneralParams: []string{"key=value"},
	}
)

var _ = JustBeforeEach(func() {
	var err error
	smServer = httptest.NewServer(createSMHandler())
	fakeAuthClient = &FakeAuthClient{AccessToken: validToken}
	client, err = NewClient(context.TODO(), &ClientConfig{URL: smServer.URL}, fakeAuthClient)
	Expect(err).ToNot(HaveOccurred())
})

func createSMHandler() http.Handler {
	mux := http.NewServeMux()
	for i := range handlerDetails {
		v := handlerDetails[i]
		mux.HandleFunc(v.Path, func(response http.ResponseWriter, req *http.Request) {
			if v.Method != req.Method {
				return
			}
			for key, value := range v.Headers {
				response.Header().Set(key, value)
			}
			authorization := req.Header.Get("Authorization")
			if authorization != "Bearer "+validToken {
				response.WriteHeader(http.StatusUnauthorized)
				response.Write([]byte(""))
				return
			}
			response.WriteHeader(v.ResponseStatusCode)
			response.Write(v.ResponseBody)
		})
	}
	return mux
}
