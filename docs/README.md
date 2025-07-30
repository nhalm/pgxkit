# Documentation

This directory contains the source documentation that automatically syncs to the GitHub wiki.

## How it works

When files in this `docs/` directory are modified and pushed to the main branch, the wiki sync workflow automatically:

1. Copies all `.md` files from `docs/` to the repository wiki
2. Maintains wiki-specific files like `_Sidebar.md` and `_Footer.md` 
3. Commits changes with a descriptive message including source SHA

This is the **single source of truth** for all documentation - edit files here, not in the wiki directly.

## Workflow

The sync is handled by `.github/workflows/wiki-sync.yml` which:
- Triggers on pushes to main affecting `docs/**` files
- Can be manually triggered via workflow dispatch
- Only commits when there are actual changes

## Files

All markdown files in this directory will be synced to the wiki:

- `Home.md` - Wiki homepage
- `Getting-Started.md` - Quick start guide
- `API-Reference.md` - Complete API documentation
- `Examples.md` - Usage examples
- `FAQ.md` - Frequently asked questions
- `Performance-Guide.md` - Performance optimization guide
- `Production-Guide.md` - Production deployment guide
- `Testing-Guide.md` - Testing strategies and utilities
- `Contributing.md` - Development contribution guide
- `_Sidebar.md` - Wiki navigation sidebar
- `_Footer.md` - Wiki footer content

## Editing

To update the wiki:
1. Edit files in this `docs/` directory
2. Commit and push to main branch
3. The workflow will automatically sync to the wiki 