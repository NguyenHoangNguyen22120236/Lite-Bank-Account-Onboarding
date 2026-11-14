
package main

import (
    "log"
    "net/http"

    "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1/onboardingv1connect"
)

func main() {
    mux := http.NewServeMux()

    handler := &onboardingv1connect.UnimplementedThirdPartyServiceHandler{}

    path, rpcHandler :=
        onboardingv1connect.NewThirdPartyServiceHandler(handler)

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

    log.Fatal(http.ListenAndServe(addr, mux))
}
