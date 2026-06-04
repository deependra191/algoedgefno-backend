package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/deependra191/algoedgefno-backend/local-rnd/kite"
)

const (
	defaultOutputPath = "/private/tmp/kite-access-token.env"
	defaultTimeout    = 30 * time.Second
	fileModeOwnerOnly = 0o600
)

const (
	exitOK    = 0
	exitError = 1
)

func main() {
	os.Exit(run())
}

func run() int {
	requestToken := flag.String("request-token", "", "request_token from the interactive Kite redirect")
	outPath := flag.String("out", defaultOutputPath, "absolute path for the generated env file")
	timeout := flag.Duration("timeout", defaultTimeout, "token exchange timeout")
	flag.Parse()

	creds, err := kite.LoadLoginCredentials()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load Kite login credentials: %v\n", err)
		return exitError
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	accessToken, err := kite.ExchangeRequestToken(ctx, creds, *requestToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "exchange request token: %v\n", err)
		return exitError
	}

	if err := writeEnvFile(*outPath, accessToken); err != nil {
		fmt.Fprintf(os.Stderr, "write env file: %v\n", err)
		return exitError
	}
	fmt.Printf("wrote %s with mode 0600; source it locally to set %s\n", *outPath, kite.EnvAccessToken)
	return exitOK
}

func writeEnvFile(path, accessToken string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("output path is required")
	}
	content := "export " + kite.EnvAccessToken + "=" + shellQuote(accessToken) + "\n"
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileModeOwnerOnly)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(fileModeOwnerOnly); err != nil {
		return err
	}
	if _, err := file.WriteString(content); err != nil {
		return err
	}
	return nil
}

func shellQuote(value string) string {
	return strconv.Quote(value)
}
