<!--
Sync Impact Report:
Version change: 1.0.0 → 1.1.0
- Expanded Principle XI (Git Best Practices) with git worktree guidance
- Added git worktree recommendation for parallel feature development
- Templates requiring updates:
  ✅ plan-template.md (references constitution in Constitution Check section - no changes needed)
  ✅ spec-template.md (no direct constitution references - no changes needed)
  ✅ tasks-template.md (references TDD practice - aligns with new principles - no changes needed)
- Follow-up TODOs: RATIFICATION_DATE marked as TODO as original adoption date unknown
-->

# Spherical Constitution

## Core Principles

### I. Test Driven Development (NON-NEGOTIABLE)

**MUST**: All development follows strict TDD methodology. Tests are written first, reviewed and approved by stakeholders, then verified to fail, and only then is implementation begun. The Red-Green-Refactor cycle is strictly enforced. Tests drive design decisions and implementation boundaries.

**Rationale**: TDD ensures correctness, prevents regressions, documents behavior, and forces clean interfaces. This discipline is non-negotiable and must be verified in all code reviews.

### II. Library First Approach

**MUST**: Every feature starts as a standalone, reusable library. Libraries MUST be self-contained, independently testable, and fully documented with clear purpose. No organizational-only libraries allowed—each library must have a concrete, standalone use case.

**Rationale**: Library-first design promotes reusability, testability, and maintainability. It forces clear interfaces and prevents tight coupling between features.

### III. CLI First Approach

**MUST**: Every library exposes functionality via a CLI interface for easy testing during development phases. Text in/out protocol: stdin/args → stdout, errors → stderr. Support both JSON and human-readable output formats for maximum flexibility.

**Rationale**: CLI interfaces enable rapid testing, automation, integration, and debugging without requiring complex test harnesses or UI frameworks. This accelerates development cycles.

### IV. Integration Testing Without Mocks

**MUST**: Integration tests use real dependencies, not mocks. Focus areas requiring integration tests: New library contract tests, Contract changes, Inter-service communication, Shared schemas. Tests must exercise actual system behavior end-to-end.

**Rationale**: Mocks hide integration issues and provide false confidence. Real integration tests catch configuration errors, compatibility issues, and environmental problems that mocks cannot detect.

### V. Real-Time Task List Updates

**MUST**: Task lists MUST be updated in real-time during development. As tasks are completed, identified, or modified, the task list documentation must immediately reflect current status.

**Rationale**: Accurate, up-to-date task lists are critical for tracking progress, coordinating team efforts, and maintaining project momentum. Stale task lists lead to confusion and wasted effort.

## Technology & Architecture

### VI. Go Programming Language

**MUST**: The Go programming language is the primary language for the codebase. All core functionality, libraries, and services MUST be implemented in Go unless explicit exception is documented and justified.

**Rationale**: Go provides excellent concurrency, performance, simplicity, and tooling. Standardizing on Go reduces cognitive load, enables code reuse, and ensures consistency across the codebase.

### VII. Modular Architecture

**MUST**: Follow best practices for modular architecture. Modules must have clear boundaries, minimal dependencies, well-defined interfaces, and adhere to single responsibility principle. Dependencies must flow in one direction to prevent circular dependencies.

**Rationale**: Modular architecture enables maintainability, testability, and parallel development. Clear boundaries reduce coupling and make the codebase easier to understand and modify.

### VIII. Module Size Limits

**MUST**: If a module exceeds 500 lines of code, it MUST be broken up into smaller sub-modules for code maintenance. Large modules indicate multiple responsibilities or excessive complexity that should be decomposed.

**Rationale**: Smaller modules are easier to understand, test, and maintain. The 500-line threshold provides a clear, measurable guideline for when refactoring is required.

### IX. Enterprise Grade Security and Architecture

**MUST**: All components MUST follow enterprise-grade security practices and architectural patterns. This includes: Secure by default configurations, Input validation and sanitization, Authentication and authorization, Encryption in transit and at rest, Audit logging, Security reviews for all changes.

**Rationale**: Enterprise-grade standards protect user data, prevent security vulnerabilities, and ensure the system can operate safely in production environments.

## Development Workflow

### X. Comprehensive Documentation

**MUST**: Create strong and water-tight documentation. All libraries, APIs, and significant features MUST include: API documentation, Usage examples, Architecture diagrams where applicable, Migration guides for breaking changes, Inline code comments for complex logic.

**Rationale**: Documentation is essential for onboarding, maintenance, and long-term sustainability. Incomplete documentation creates technical debt and slows development velocity.

### XI. Git Best Practices

**MUST**: Follow strong Git best practices for code management and version control. All major features MUST be created on their own branches and merged with main only after explicit confirmation that everything is tested and working. This includes: Feature branches for all non-trivial changes, Clear commit messages following conventional commit format, Code review required before merge, All tests passing before merge approval.

**SHOULD**: Use Git worktrees for parallel feature development. Worktrees enable simultaneous work on multiple features without branch switching, support parallel testing across features, and maintain clean working directory states per feature. Worktrees MUST be created for feature branches when parallel development is required and SHOULD be cleaned up after merge completion.

**Rationale**: Git best practices ensure code quality, enable safe experimentation, provide audit trails, and prevent breaking changes from reaching main branch. Worktrees enhance productivity by enabling parallel development workflows, reducing context switching overhead, and maintaining isolated feature environments for independent testing and validation.

## Governance

**Constitution Supremacy**: This constitution supersedes all other development practices, conventions, and guidelines. Any deviation requires explicit documentation, justification, and approval through the amendment process.

**Compliance**: All code reviews, pull requests, and development activities MUST verify compliance with these principles. Violations must be addressed before code can be merged.

**Amendments**: Amendments to this constitution require:

1. Documentation of the proposed change and rationale
2. Review and approval process
3. Migration plan for any breaking changes
4. Version bump according to semantic versioning:
   - **MAJOR**: Backward incompatible governance or principle removals/redefinitions
   - **MINOR**: New principle added or materially expanded guidance
   - **PATCH**: Clarifications, wording improvements, typo fixes

**Complexity Justification**: Any decision that increases complexity or violates a principle must be explicitly justified with:

- Why the complexity is necessary
- What simpler alternatives were considered and rejected
- Long-term maintenance impact

**Version**: 1.1.0 | **Ratified**: TODO(RATIFICATION_DATE): Original adoption date unknown | **Last Amended**: 2025-11-21
