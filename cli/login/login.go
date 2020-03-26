package login

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"

	"github.com/kelda-inc/blimp/cli/authstore"
	"github.com/kelda-inc/blimp/pkg/auth"
)

func New() *cobra.Command {
	return &cobra.Command{
		Use: "login",
		Run: func(_ *cobra.Command, _ []string) {
			token, err := getAuthToken()
			if err != nil {
				log.WithError(err).Fatal("Failed to login")
			}
			fmt.Println("Successfully logged in")

			// TODO: Store in OS's encrypted storage rather than in regular file.
			store, err := authstore.New()
			if err != nil {
				log.WithError(err).Fatal("Failed to parse existing Kelda Blimp credentials")
			}

			store.AuthToken = token
			if err := store.Save(); err != nil {
				log.WithError(err).Fatal("Failed to update local Kelda Blimp credentials")
			}
		},
	}
}

func getAuthToken() (string, error) {
	oauthConf := &oauth2.Config{
		ClientID:    auth.ClientID,
		Endpoint:    auth.Endpoint,
		RedirectURL: fmt.Sprintf("http://%s%s", auth.RedirectHost, auth.RedirectPath),
		Scopes: []string{
			"openid",
			"email",
		},
	}

	challenge, verifier, err := makeVerifier()
	if err != nil {
		return "", fmt.Errorf("create verifier for oauth handshake", err)
	}

	codeRespChan := make(chan *http.Request, 1)
	serveMux := http.NewServeMux()
	serveMux.HandleFunc(auth.RedirectPath, func(w http.ResponseWriter, r *http.Request) {
		codeRespChan <- r
		// TODO: Automatically close the window with Javascript.
		fmt.Fprintln(w, "You can close this window now.")
	})

	server := http.Server{
		Handler: serveMux,
		Addr:    auth.RedirectHost,
	}
	go server.ListenAndServe()
	defer server.Close()

	// TODO: Set and check state.
	authURL := oauthConf.AuthCodeURL("state",
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	fmt.Printf("Open the following URL in your browser: %s\n", authURL)

	codeResp := <-codeRespChan

	err = codeResp.ParseForm()
	if err != nil {
		return "", fmt.Errorf("parse form: %w", err)
	}

	// TODO: Test bad creds.
	code := codeResp.FormValue("code")
	if code == "" {
		return "", errors.New("no auth code")
	}

	tok, err := oauthConf.Exchange(context.Background(), code,
		oauth2.SetAuthURLParam("code_verifier", verifier),
	)
	if err != nil {
		return "", fmt.Errorf("exchange auth code: %w", err)
	}

	idToken, ok := tok.Extra("id_token").(string)
	if !ok {
		return "", errors.New("missing id token")
	}
	return idToken, nil
}

func makeVerifier() (challenge string, verifier string, err error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", "", err
	}

	// The verifier can only contain the characters A-Z, a-z, 0-9, and the
	// following punctuation characters: -._~
	verifier = base64Encode(randomBytes)
	h := sha256.New()
	h.Write([]byte(verifier))
	return base64Encode(h.Sum(nil)), verifier, nil
}

func base64Encode(buf []byte) string {
	return strings.Replace(base64.URLEncoding.EncodeToString(buf), "=", "", -1)
}
