# Handoff to Claude Code

Copy and paste this prompt to your Claude Code instance:

---

I need you to implement a new MVP project for Bad.no - a CLI tool for product operations.

**Project location**: `/Users/ermankuplu/Building/bad-no-ops`

**Read these documentation files first:**
1. `docs/PROJECT.md` - Business context and requirements
2. `docs/BRIEF.md` - Implementation details

**Key constraints:**
- This is a **standalone CLI tool**
- Use the Cobra CLI framework
- File-based storage only (no database)
- No authentication needed

**Start with:**
1. Read both documentation files
2. Confirm your understanding of the scope
3. Create the project structure
4. Implement in this order:
   - Product parser (Matrixify CSV)
   - Tiger.nl product matcher
   - Image fetcher
   - Image resizer with center-crop

**Test data:**
For initial testing, we'll use a Matrixify export of Tiger brand products from Bad.no's Shopify store. If no test data is available, create a mock CSV with 10 sample products.

Ask me any clarifying questions before starting implementation.
