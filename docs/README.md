# Tunnox Core Documentation

This directory contains active documentation for the Tunnox Core project.

## Documentation Structure

### Core Documentation
- **[TUNNOX_CODING_STANDARDS.md](TUNNOX_CODING_STANDARDS.md)** - Coding standards and best practices
- **[ARCHITECTURE_DESIGN_V2.2.md](ARCHITECTURE_DESIGN_V2.2.md)** - Current system architecture
- **[MANAGEMENT_API.md](MANAGEMENT_API.md)** - Management REST API documentation

### Release & Deployment
- **[RELEASE_PROCESS.md](RELEASE_PROCESS.md)** - Detailed release process
- **[RELEASE_QUICK_START.md](RELEASE_QUICK_START.md)** - Quick release guide

### Recent Fixes & Enhancements
- **[fragment-group-limit-fix.md](fragment-group-limit-fix.md)** - HTTP Poll fragment group limit fix (2025-12-09)
- **[error-handling-migration-status.md](error-handling-migration-status.md)** - Error handling migration completion report (2025-12-09)

### Architecture Reference
- **[architecture/terminology.md](architecture/terminology.md)** - Architecture terminology and concepts

## Archive

Historical design documents, implementation plans, and code reviews have been moved to the `archive/` directory:

- `archive/implemented/` - Completed design and implementation plans
- `archive/code-reviews/` - Historical code review documents

See [archive/README.md](archive/README.md) for details.

## Contributing

When creating new documentation:

1. **Design Documents**: Create in the main `docs/` directory during the design phase
2. **Implementation Plans**: Create alongside design documents
3. **Post-Implementation**: After successful implementation and testing:
   - Update relevant architecture/API documentation
   - Move implementation plans to `archive/implemented/`
   - Create a summary document of the changes (like `fragment-group-limit-fix.md`)

## Documentation Guidelines

- Use clear, concise markdown
- Include code examples where appropriate
- Date documents for historical reference
- Link to related code files using relative paths
- Follow the structure: Problem → Analysis → Solution → Results
