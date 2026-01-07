package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/badno/badops/internal/images"
	"github.com/badno/badops/internal/matcher"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

var (
	fetchLimit  int
	resizeSize  int
	downloadNew bool
)

var imagesCmd = &cobra.Command{
	Use:   "images",
	Short: "Manage product images",
	Long:  `Fetch, resize, and process product images.`,
}

var fetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch images from Tiger.nl",
	Long:  `Download product images from matched Tiger.nl product pages.`,
	RunE:  runFetch,
}

var resizeCmd = &cobra.Command{
	Use:   "resize",
	Short: "Resize images to square format",
	Long:  `Center-crop and resize images to square format (default 800x800).`,
	RunE:  runResize,
}

var compareCmd = &cobra.Command{
	Use:   "compare",
	Short: "Compare images between bad.no and Tiger.nl",
	Long:  `Check how many images each product has on bad.no vs Tiger.nl and identify new images to download.`,
	RunE:  runCompare,
}

func init() {
	fetchCmd.Flags().IntVarP(&fetchLimit, "limit", "l", 0, "Limit number of images to fetch (0 = all)")
	fetchCmd.Flags().BoolVar(&downloadNew, "new-only", false, "Only download new images not already on bad.no")
	resizeCmd.Flags().IntVarP(&resizeSize, "size", "s", 800, "Target size for square images")

	imagesCmd.AddCommand(fetchCmd)
	imagesCmd.AddCommand(resizeCmd)
	imagesCmd.AddCommand(compareCmd)
}

func runFetch(cmd *cobra.Command, args []string) error {
	header := color.New(color.FgCyan, color.Bold)
	success := color.New(color.FgGreen)

	if downloadNew {
		return runFetchNewOnly()
	}

	header.Println("\n  FETCHING IMAGES FROM TIGER.NL")
	fmt.Println("  " + strings.Repeat("─", 40))
	fmt.Println()

	// Get real Tiger.nl image URLs (hardcoded for demo)
	fetcher := images.NewFetcher()
	imageURLs := fetcher.GetDemoImageURLs()

	limit := len(imageURLs)
	if fetchLimit > 0 && fetchLimit < limit {
		limit = fetchLimit
	}
	imageURLs = imageURLs[:limit]

	color.Yellow("  Found %d images to download\n\n", len(imageURLs))

	// Progress bar
	bar := progressbar.NewOptions(len(imageURLs),
		progressbar.OptionSetDescription("  Downloading images"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        color.GreenString("█"),
			SaucerHead:    color.GreenString("█"),
			SaucerPadding: "░",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionShowCount(),
		progressbar.OptionShowBytes(true),
	)

	downloaded := 0
	failed := 0
	results := make([]struct {
		url    string
		path   string
		status string
		size   string
	}, 0)

	for _, img := range imageURLs {
		path, size, err := fetcher.Download(img.URL, img.SKU)
		bar.Add(1)

		if err != nil {
			results = append(results, struct {
				url    string
				path   string
				status string
				size   string
			}{img.URL, "", "failed", "-"})
			failed++
		} else {
			results = append(results, struct {
				url    string
				path   string
				status string
				size   string
			}{img.URL, path, "success", size})
			downloaded++
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Println()
	fmt.Println()

	// Display results table
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"SKU", "File", "Size", "Status"})
	table.SetBorder(false)
	table.SetHeaderColor(
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
	)

	for i, r := range results {
		sku := imageURLs[i].SKU
		filename := "-"
		if r.path != "" {
			parts := strings.Split(r.path, "/")
			filename = parts[len(parts)-1]
		}
		status := color.GreenString("downloaded")
		if r.status == "failed" {
			status = color.RedString("failed")
		}
		table.Append([]string{sku, filename, r.size, status})
	}
	table.Render()
	fmt.Println()

	// Summary
	if downloaded > 0 {
		success.Printf("  ✓ Downloaded %d images to output/originals/\n", downloaded)
	}
	if failed > 0 {
		color.Red("  ✗ Failed to download %d images\n", failed)
	}
	fmt.Println()

	return nil
}

func runFetchNewOnly() error {
	header := color.New(color.FgCyan, color.Bold)
	success := color.New(color.FgGreen)

	header.Println("\n  FETCHING NEW IMAGES FROM TIGER.NL")
	fmt.Println("  " + strings.Repeat("─", 45))
	fmt.Println()

	// Load products from state
	products, err := loadState()
	if err != nil || len(products) == 0 {
		color.Yellow("  No products loaded. Run 'badops products parse' first.")
		return nil
	}

	color.Yellow("  Scanning %d products for new images...\n\n", len(products))

	// Create scraper and fetcher
	scraper := matcher.NewTigerScraper()
	fetcher := images.NewFetcher()

	// First pass: find all new images
	type newImage struct {
		sku string
		url string
		idx int
	}
	var newImages []newImage

	// Progress bar for scanning
	scanBar := progressbar.NewOptions(len(products),
		progressbar.OptionSetDescription("  Scanning Tiger.nl"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        color.CyanString("█"),
			SaucerHead:    color.CyanString("█"),
			SaucerPadding: "░",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionShowCount(),
	)

	for _, p := range products {
		existingCount := len(p.ExistingImages)

		// Find product on Tiger.nl
		tigerProduct, err := scraper.FindProduct(p.Name)
		if err == nil && tigerProduct != nil {
			// Skip first N images (already on bad.no), take the rest
			for i, imgURL := range tigerProduct.ImageURLs {
				if i >= existingCount {
					newImages = append(newImages, newImage{
						sku: p.SKU,
						url: imgURL,
						idx: i - existingCount + 1,
					})
				}
			}
		}
		scanBar.Add(1)
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Println()
	fmt.Println()

	if len(newImages) == 0 {
		color.Yellow("  No new images found. All products are up to date.")
		fmt.Println()
		return nil
	}

	// Apply limit if specified
	if fetchLimit > 0 && fetchLimit < len(newImages) {
		newImages = newImages[:fetchLimit]
	}

	color.Green("  Found %d new images to download\n\n", len(newImages))

	// Progress bar for downloading
	downloadBar := progressbar.NewOptions(len(newImages),
		progressbar.OptionSetDescription("  Downloading new images"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        color.GreenString("█"),
			SaucerHead:    color.GreenString("█"),
			SaucerPadding: "░",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionShowCount(),
		progressbar.OptionShowBytes(true),
	)

	downloaded := 0
	failed := 0
	results := make([]struct {
		sku    string
		url    string
		path   string
		status string
		size   string
		idx    int
	}, 0)

	for _, img := range newImages {
		// Create filename with index for new images
		filename := fmt.Sprintf("%s_new_%d", img.sku, img.idx)
		path, size, err := fetcher.Download(img.url, filename)
		downloadBar.Add(1)

		if err != nil {
			results = append(results, struct {
				sku    string
				url    string
				path   string
				status string
				size   string
				idx    int
			}{img.sku, img.url, "", "failed", "-", img.idx})
			failed++
		} else {
			results = append(results, struct {
				sku    string
				url    string
				path   string
				status string
				size   string
				idx    int
			}{img.sku, img.url, path, "success", size, img.idx})
			downloaded++
		}
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Println()
	fmt.Println()

	// Display results table
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"SKU", "New #", "File", "Size", "Status"})
	table.SetBorder(false)
	table.SetHeaderColor(
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
	)

	for _, r := range results {
		filename := "-"
		if r.path != "" {
			parts := strings.Split(r.path, "/")
			filename = parts[len(parts)-1]
		}
		status := color.GreenString("downloaded")
		if r.status == "failed" {
			status = color.RedString("failed")
		}
		table.Append([]string{r.sku, fmt.Sprintf("+%d", r.idx), filename, r.size, status})
	}
	table.Render()
	fmt.Println()

	// Summary
	if downloaded > 0 {
		success.Printf("  ✓ Downloaded %d NEW images to output/originals/\n", downloaded)
	}
	if failed > 0 {
		color.Red("  ✗ Failed to download %d images\n", failed)
	}
	fmt.Println()

	return nil
}

func runResize(cmd *cobra.Command, args []string) error {
	header := color.New(color.FgCyan, color.Bold)
	success := color.New(color.FgGreen)

	header.Println("\n  RESIZING IMAGES TO SQUARE FORMAT")
	fmt.Println("  " + strings.Repeat("─", 40))
	fmt.Println()

	resizer := images.NewResizer()
	imagesToResize, err := resizer.FindOriginals()
	if err != nil || len(imagesToResize) == 0 {
		color.Yellow("  No images found in output/originals/")
		color.Yellow("  Run 'badops images fetch' first.")
		return nil
	}

	color.Yellow("  Found %d images to resize to %dx%d\n\n", len(imagesToResize), resizeSize, resizeSize)

	// Progress bar
	bar := progressbar.NewOptions(len(imagesToResize),
		progressbar.OptionSetDescription("  Resizing images"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        color.GreenString("█"),
			SaucerHead:    color.GreenString("█"),
			SaucerPadding: "░",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionShowCount(),
	)

	resized := 0
	failed := 0
	results := make([]struct {
		source string
		dest   string
		status string
	}, 0)

	for _, imgPath := range imagesToResize {
		destPath, err := resizer.ResizeSquare(imgPath, resizeSize)
		bar.Add(1)

		if err != nil {
			results = append(results, struct {
				source string
				dest   string
				status string
			}{imgPath, "", "failed"})
			failed++
		} else {
			results = append(results, struct {
				source string
				dest   string
				status string
			}{imgPath, destPath, "success"})
			resized++
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Println()
	fmt.Println()

	// Display results table
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Original", "Resized", "Status"})
	table.SetBorder(false)
	table.SetHeaderColor(
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
	)

	for _, r := range results {
		sourceParts := strings.Split(r.source, "/")
		sourceFile := sourceParts[len(sourceParts)-1]

		destFile := "-"
		if r.dest != "" {
			destParts := strings.Split(r.dest, "/")
			destFile = destParts[len(destParts)-1]
		}

		status := color.GreenString("resized")
		if r.status == "failed" {
			status = color.RedString("failed")
		}
		table.Append([]string{sourceFile, destFile, status})
	}
	table.Render()
	fmt.Println()

	// Summary
	if resized > 0 {
		success.Printf("  ✓ Resized %d images to output/resized/%d/\n", resized, resizeSize)
	}
	if failed > 0 {
		color.Red("  ✗ Failed to resize %d images\n", failed)
	}
	fmt.Println()

	return nil
}

func runCompare(cmd *cobra.Command, args []string) error {
	header := color.New(color.FgCyan, color.Bold)
	success := color.New(color.FgGreen)

	header.Println("\n  COMPARING IMAGES: BAD.NO vs TIGER.NL")
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Println()

	// Load products from state
	products, err := loadState()
	if err != nil || len(products) == 0 {
		color.Yellow("  No products loaded. Run 'badops products parse' first.")
		return nil
	}

	color.Yellow("  Checking %d products...\n\n", len(products))

	// Create scraper
	scraper := matcher.NewTigerScraper()

	// Progress bar
	bar := progressbar.NewOptions(len(products),
		progressbar.OptionSetDescription("  Scanning Tiger.nl"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        color.GreenString("█"),
			SaucerHead:    color.GreenString("█"),
			SaucerPadding: "░",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionShowCount(),
	)

	type comparison struct {
		sku         string
		name        string
		badnoCount  int
		tigerCount  int
		newCount    int
		tigerImages []string
	}

	var comparisons []comparison
	totalNew := 0

	for _, p := range products {
		badnoCount := len(p.ExistingImages)

		// Try to find product on Tiger.nl
		tigerCount := 0
		var tigerImages []string

		tigerProduct, err := scraper.FindProduct(p.Name)
		if err == nil && tigerProduct != nil {
			tigerCount = len(tigerProduct.ImageURLs)
			tigerImages = tigerProduct.ImageURLs
		}

		newCount := 0
		if tigerCount > badnoCount {
			newCount = tigerCount - badnoCount
		}
		totalNew += newCount

		comparisons = append(comparisons, comparison{
			sku:         p.SKU,
			name:        p.Name,
			badnoCount:  badnoCount,
			tigerCount:  tigerCount,
			newCount:    newCount,
			tigerImages: tigerImages,
		})

		bar.Add(1)
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Println()
	fmt.Println()

	// Display comparison table
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"SKU", "Product", "Bad.no", "Tiger.nl", "New"})
	table.SetBorder(false)
	table.SetHeaderColor(
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
		tablewriter.Colors{tablewriter.Bold, tablewriter.FgCyanColor},
	)

	for _, c := range comparisons {
		name := c.name
		if len(name) > 25 {
			name = name[:22] + "..."
		}

		badnoStr := fmt.Sprintf("%d", c.badnoCount)
		if c.badnoCount == 0 {
			badnoStr = color.RedString("0")
		}

		tigerStr := fmt.Sprintf("%d", c.tigerCount)
		if c.tigerCount == 0 {
			tigerStr = color.YellowString("?")
		}

		newStr := fmt.Sprintf("%d", c.newCount)
		if c.newCount > 0 {
			newStr = color.GreenString("+%d", c.newCount)
		}

		table.Append([]string{c.sku, name, badnoStr, tigerStr, newStr})
	}
	table.Render()
	fmt.Println()

	// Summary
	productsWithNew := 0
	for _, c := range comparisons {
		if c.newCount > 0 {
			productsWithNew++
		}
	}

	if totalNew > 0 {
		success.Printf("  ✓ Found %d new images across %d products\n", totalNew, productsWithNew)
		color.Yellow("  → Run 'badops images fetch --new-only' to download them\n")
	} else {
		color.Yellow("  All products have matching images\n")
	}
	fmt.Println()

	return nil
}
