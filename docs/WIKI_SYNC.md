# Wiki Synchronization

This document explains the automated wiki synchronization system implemented for the pgxkit repository.

## Overview

The pgxkit repository features bi-directional synchronization between the `wiki-templates/` directory and the GitHub Wiki, enabling seamless collaboration between developers who prefer editing documentation in the repository and those who prefer the wiki interface.

## Architecture

### Source Directory: `wiki-templates/`
- Contains all wiki page templates in Markdown format
- Serves as the authoritative source for documentation structure
- Files are automatically synced to the GitHub Wiki when pushed to `main`

### GitHub Wiki
- Provides user-friendly editing interface
- Changes made directly in the wiki automatically sync back to `wiki-templates/`
- Supports all GitHub Wiki features (editing, history, etc.)

## Synchronization Triggers

### 1. Repository → Wiki Sync
**Trigger**: Push to `main` branch with changes in `wiki-templates/`

**Process**:
1. Detects changes in `wiki-templates/**` files
2. Updates timestamp in `Home.md`
3. Syncs all files to GitHub Wiki
4. Preserves wiki structure and navigation

### 2. Wiki → Repository Sync
**Trigger**: `gollum` event (direct wiki edits)

**Process**:
1. Detects wiki page edits
2. Syncs changes back to `wiki-templates/`
3. Resets timestamp placeholder in `Home.md`
4. Commits changes to `main` branch

## File Mapping

| Repository File | Wiki Page |
|---|---|
| `wiki-templates/Home.md` | `Home` |
| `wiki-templates/Getting-Started.md` | `Getting-Started` |
| `wiki-templates/API-Reference.md` | `API-Reference` |
| `wiki-templates/Examples.md` | `Examples` |

## Timestamp Management

The `Home.md` template includes an automatic timestamp feature:

```markdown
*Last updated: [Auto-generated timestamp will be inserted here during sync]*
```

This placeholder is automatically replaced with the current UTC timestamp when syncing from repository to wiki, and reset to the placeholder when syncing from wiki to repository.

## Bot Configuration

The synchronization is handled by the `pgxkit-bot` account with the following configuration:
- **Name**: `pgxkit-bot`
- **Email**: `actions@github.com`
- **Permissions**: Repository write access via `GITHUB_TOKEN`

## Workflow Details

### GitHub Action: `.github/workflows/sync-wiki.yml`

The workflow includes two jobs:

1. **sync-docs-to-wiki**: Handles repository → wiki synchronization
2. **sync-wiki-to-docs**: Handles wiki → repository synchronization

### Required Permissions

The workflow uses the default `GITHUB_TOKEN` which provides:
- Repository read/write access
- Wiki read/write access
- Ability to trigger on `gollum` events

## Usage Guidelines

### For Documentation Contributors

1. **Preferred Method**: Edit files in `wiki-templates/` directory
2. **Direct Wiki Editing**: Also supported, changes will sync back automatically
3. **Commit Messages**: Use `[skip-ci]` to prevent automatic sync if needed

### Adding New Wiki Pages

1. Create a new `.md` file in `wiki-templates/`
2. Follow the naming convention (use hyphens for spaces)
3. Update navigation links in `Home.md`
4. Push to `main` branch to sync to wiki

### Handling Conflicts

- The system handles most conflicts automatically
- For complex conflicts, manual intervention may be required
- Check GitHub Actions logs for sync status and errors

## Limitations

1. **File Deletion**: Wiki page deletions don't automatically sync back due to GitHub API limitations
2. **Binary Files**: Only Markdown files are synchronized
3. **Wiki Uploads**: Files uploaded directly to wiki won't sync to repository

## Troubleshooting

### Sync Not Working
1. Check GitHub Actions tab for workflow runs
2. Verify repository permissions
3. Ensure GitHub Wiki is enabled

### Timestamp Issues
1. Check that `Home.md` contains the timestamp placeholder
2. Verify the sed commands in the workflow are working correctly

### Conflicts
1. Check for merge conflicts in the repository
2. Manually resolve conflicts and commit
3. Re-trigger sync by making a small edit

## Monitoring

Monitor synchronization status through:
- GitHub Actions workflow runs
- Git commit history
- Wiki page history
- Repository file change history

This automated system ensures that documentation stays synchronized while allowing contributors to use their preferred editing method. 