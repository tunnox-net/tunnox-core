# Tunnox Core Documentation

This directory contains active documentation for the Tunnox Core project.

## Core Documentation

- **[TUNNOX_CODING_STANDARDS.md](TUNNOX_CODING_STANDARDS.md)** - Coding standards and best practices (must read!)
- **[ARCHITECTURE_DESIGN_V2.2.md](ARCHITECTURE_DESIGN_V2.2.md)** - Current system architecture
- **[MANAGEMENT_API.md](MANAGEMENT_API.md)** - Management REST API documentation

## Release & Deployment

- **[RELEASE_PROCESS.md](RELEASE_PROCESS.md)** - Detailed release process
- **[RELEASE_QUICK_START.md](RELEASE_QUICK_START.md)** - Quick release guide

## Recent Fixes

- **[fragment-group-limit-fix.md](fragment-group-limit-fix.md)** - HTTP Poll fragment group limit fix (2025-12-09)

## Architecture Reference

- **[architecture/terminology.md](architecture/terminology.md)** - Architecture terminology and concepts

## Archive

Historical code review documents are in the `archive/` directory:

- `archive/code-reviews/` - Historical code review documents

See [archive/README.md](archive/README.md) for details.

## Documentation Philosophy

**Completed = Deleted**

Once a design is implemented and tested, we delete the design document. The code itself is the documentation. This keeps the docs directory clean and focused on:

1. **Active Standards** - Coding standards, architecture guidelines
2. **Reference Docs** - API documentation, release processes
3. **Recent Fixes** - Documentation of notable bug fixes (for historical context)

We do NOT keep:
- ❌ Completed implementation plans
- ❌ Finished task checklists
- ❌ Migration guides for completed migrations
- ❌ Design documents for implemented features

## Contributing Documentation

### When to Create Docs

- **Architecture Changes**: Update ARCHITECTURE_DESIGN_V2.2.md
- **API Changes**: Update MANAGEMENT_API.md
- **Coding Standards**: Update TUNNOX_CODING_STANDARDS.md
- **Bug Fixes**: Create a brief fix document if the issue was complex (like fragment-group-limit-fix.md)

### When NOT to Create Docs

- ❌ Implementation plans (just implement it)
- ❌ Task checklists (use issue tracker)
- ❌ Design documents (code is the design)

Keep documentation minimal, accurate, and up-to-date!
