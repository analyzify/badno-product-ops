package cmd

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var crawlTestCmd = &cobra.Command{
	Use:   "crawltest",
	Short: "Test Web Bot Auth signature against bad.no",
	Long:  `Tests if the Shopify Web Bot Auth signature allows crawling bad.no without rate limiting.`,
	RunE:  runCrawlTest,
}

func init() {
	rootCmd.AddCommand(crawlTestCmd)
	crawlTestCmd.Flags().IntP("requests", "n", 10, "Number of test requests to make")
	crawlTestCmd.Flags().IntP("delay", "d", 100, "Delay between requests in milliseconds")
	crawlTestCmd.Flags().Bool("no-auth", false, "Test without Web Bot Auth headers (to compare)")
}

// Web Bot Auth credentials for bad.no
const (
	signature      = `sig1=:0h2gt3+g1RGZ5203v+yqeJhTVikBwPVxktghzphCueruEsVjtHgdDE63+BTRTkwXWZ4EKlK/kkTFdkoaULz1DA==:`
	signatureInput = `sig1=("@authority" "signature-agent");keyid="FZhBoubzJDfpJUPLUmyg5OTEuggRhXFStBseUXjMACE";nonce="qHUkCMc/mgDUHTT5hbbk60pIpl5gZ3VCAKdV2aiYOmIZVSkcl7FsXdhJfKTgPdfEOXCs/PBisZ+Too3wuIWSyg==";tag="web-bot-auth";created=1768589015;expires=1776365015`
	signatureAgent = `"https://shopify.com"`
)

func runCrawlTest(cmd *cobra.Command, args []string) error {
	numRequests, _ := cmd.Flags().GetInt("requests")
	delay, _ := cmd.Flags().GetInt("delay")
	noAuth, _ := cmd.Flags().GetBool("no-auth")

	testURLs := []string{
		"https://bad.no/",
		"https://bad.no/collections/all",
		"https://bad.no/products.json?limit=1",
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	success := color.New(color.FgGreen).SprintFunc()
	failure := color.New(color.FgRed).SprintFunc()
	info := color.New(color.FgCyan).SprintFunc()

	fmt.Println()
	if noAuth {
		fmt.Printf("%s Testing WITHOUT Web Bot Auth headers\n", info("ℹ"))
	} else {
		fmt.Printf("%s Testing WITH Web Bot Auth headers\n", info("ℹ"))
	}
	fmt.Printf("%s Making %d requests with %dms delay\n\n", info("ℹ"), numRequests, delay)

	var successCount, failCount, rateLimitCount int

	for i := 0; i < numRequests; i++ {
		url := testURLs[i%len(testURLs)]

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		// Set standard headers
		req.Header.Set("User-Agent", "BadOps-Crawler/1.0 (https://bad.no)")
		req.Header.Set("Accept", "text/html,application/json")

		// Add Web Bot Auth headers if enabled
		if !noAuth {
			req.Header.Set("Signature", signature)
			req.Header.Set("Signature-Input", signatureInput)
			req.Header.Set("Signature-Agent", signatureAgent)
		}

		start := time.Now()
		resp, err := client.Do(req)
		elapsed := time.Since(start)

		if err != nil {
			fmt.Printf("[%d/%d] %s %s - %s\n", i+1, numRequests, failure("ERR"), url, err.Error())
			failCount++
		} else {
			// Read a bit of the body to ensure connection is complete
			io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
			resp.Body.Close()

			status := resp.StatusCode
			switch {
			case status == 200:
				fmt.Printf("[%d/%d] %s %s (%dms)\n", i+1, numRequests, success("200"), url, elapsed.Milliseconds())
				successCount++
			case status == 429:
				fmt.Printf("[%d/%d] %s %s - RATE LIMITED\n", i+1, numRequests, failure("429"), url)
				rateLimitCount++
			case status == 403:
				fmt.Printf("[%d/%d] %s %s - FORBIDDEN (bot blocked)\n", i+1, numRequests, failure("403"), url)
				failCount++
			default:
				fmt.Printf("[%d/%d] %s %s\n", i+1, numRequests, info(fmt.Sprintf("%d", status)), url)
				if status >= 400 {
					failCount++
				} else {
					successCount++
				}
			}
		}

		if i < numRequests-1 {
			time.Sleep(time.Duration(delay) * time.Millisecond)
		}
	}

	// Summary
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Results: %s success, %s failed, %s rate limited\n",
		success(fmt.Sprintf("%d", successCount)),
		failure(fmt.Sprintf("%d", failCount)),
		failure(fmt.Sprintf("%d", rateLimitCount)))

	if rateLimitCount > 0 {
		fmt.Printf("\n%s Rate limiting detected! The signature may be invalid or expired.\n", failure("✗"))
		return fmt.Errorf("rate limiting detected")
	}

	if failCount == 0 && successCount > 0 {
		fmt.Printf("\n%s All requests passed! Web Bot Auth is working correctly.\n", success("✓"))
	}

	return nil
}
