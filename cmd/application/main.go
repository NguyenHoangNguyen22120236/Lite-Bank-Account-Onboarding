package main

import (
    "log"
    "net/http"

    "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/gen/onboarding/v1/onboardingv1connect"
)

func main() {
    mux := http.NewServeMux()

    handler := &onboardingv1connect.UnimplementedApplicationServiceHandler{}

    path, rpcHandler :=
        onboardingv1connect.NewApplicationServiceHandler(handler)

    mux.Handle(path, rpcHandler)

    mux.HandleFunc("/health", func(
        w http.ResponseWriter,
        r *http.Request,
    ) {
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("application service healthy"))
    })

    addr := ":8080"

    log.Printf("ApplicationService running on %s", addr)

    log.Fatal(http.ListenAndServe(addr, mux))
}