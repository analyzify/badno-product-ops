# Claude Code Prompt

Copy this prompt when running Claude Code in this folder:

---

I need a **working demo** of a CLI tool for Bad.no's product team. This is for a customer demo, so prioritize visual polish and a smooth experience over full functionality.

**Read first:**
- `docs/PROJECT.md` - Business context
- `docs/BRIEF.md` - Technical structure

## What I need for the demo

### 1. CLI that looks professional
- Nice colored output, progress indicators
- Clear success/error messages
- Help text that explains what each command does

### 2. Working flow with mock data
Create `testdata/tiger-sample.csv` with 10 fake Tiger products (realistic product names like "Tiger Bathroom Shelf Chrome", "Tiger Toilet Brush Boston", etc.)

### 3. Commands that actually do something

```bash
# This should parse CSV and show nice output
badops products parse testdata/tiger-sample.csv

# This should "match" products (can be mocked/simulated matching)
# Show progress bar, confidence scores, matched URLs
badops products match

# This should actually download a few real images from Tiger.nl
# Pick 2-3 real Tiger products, hardcode their URLs if needed
badops images fetch --limit 3

# This should actually resize images to square with center-crop
badops images resize --size 800
```

### 4. Visual output matters
- Use colors (green for success, yellow for warnings, red for errors)
- Show progress bars for longer operations
- Print a summary table at the end of each command
- Save a simple JSON report to `output/report.json`

### 5. Actual image processing
The resize command must actually work:
- Download real images (even if just 2-3 hardcoded Tiger.nl URLs)
- Center-crop to square
- Save to `output/originals/` and `output/resized/800/`

## Tech choices
- Go with Cobra CLI
- Use `github.com/fatih/color` for colored output
- Use `github.com/schollz/progressbar/v3` for progress bars
- Use `github.com/disintegration/imaging` for image processing
- Use `github.com/olekukonko/tablewriter` for nice tables

## Time constraint
Keep it simple. Mock what you need to mock. But make the demo flow smooth and impressive. I want to show: parse → match → fetch → resize in one session.

Start by creating the project structure and go.mod, then implement each command.
