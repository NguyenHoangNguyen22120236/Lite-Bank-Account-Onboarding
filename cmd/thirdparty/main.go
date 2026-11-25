package main

import (
	"log"
	"net/http"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1/onboardingv1connect"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/thirdparty/handler"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/thirdparty/mock"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/thirdparty/service"
)

func main() {
	mux := http.NewServeMux()

	thirdPartyHandler := handler.New(service.New(mock.NewIdentityVerifier()))

	path, rpcHandler :=
		onboardingv1connect.NewThirdPartyServiceHandler(thirdPartyHandler)

	mux.Handle(path, rpcHandler)

	mux.HandleFunc("/health", func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("third-party service healthy"))
	})

	addr := ":8081"

	log.Printf("ThirdPartyService running on %s", addr)

	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		Protocols:         protocols,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}
