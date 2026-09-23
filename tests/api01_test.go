package out_test

import (
	"adl_testing/generated/api01/api"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adl-lang/adl-go/adl"
)

type api01impl struct {
}

// Login implements [api.Endpoints_Service].
func (a *api01impl) Login(ctx context.Context, req api.LoginReq) (api.LoginResp, error) {
	return api.Make_LoginResp_user_token("1234567890"), nil
}

// WhoAmI implements [api.Endpoints_Service].
func (a *api01impl) WhoAmI(ctx context.Context) (api.WhoAmIResp, error) {
	return api.MakeAll_WhoAmIResp("slartibartfast"), nil
}

var (
	impl api.Endpoints_Service = &api01impl{}
	eps  api.Endpoints         = api.Make_Endpoints()
)

func TestApi01(t *testing.T) {
	mux := &http.ServeMux{}
	api.Register_Endpoints(mux, impl)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	{
		sr := strings.NewReader(`{"username": "", "password": ""}`)
		res, err := http.Post(ts.URL+eps.Login.Path, "", sr)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != `{"user_token":"1234567890"}` {
			t.Errorf("got '%s'", body)
		}
		dec := adl.CreateJsonDecodeBinding(eps.Login.RespType, adl.RESOLVER)
		lresp := &api.LoginResp{}
		if err = dec.Decode(bytes.NewReader(body), lresp); err != nil {
			t.Errorf("can't decode resp. err %v", err)
		}
		if token, ok := lresp.Cast_user_token(); !ok {
			t.Errorf("wrong branch. %v", lresp)
		} else {
			if token != "1234567890" {
				t.Errorf("token ! '%s'", token)
			}
		}
	}
	{
		res, err := http.Get(ts.URL + eps.WhoAmI.Path)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != `{"username":"slartibartfast"}` {
			t.Errorf("got '%s'", body)
		}
	}
}
