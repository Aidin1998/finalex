# CODE REVIEW STANDARDS

This document defines the code review standards for the PinCEX Unified codebase. All contributors and reviewers must follow these guidelines to ensure code quality, maintainability, and security.

---

## 1. General Principles
- All code must be reviewed by at least one other engineer before merging.
- Reviews should be constructive, respectful, and focused on code quality and business requirements.
- All automated tests and linters must pass before approval.

---

## 2. Go Code Checklist
- [ ] Follows Go style conventions (`gofmt`, `golint`, idiomatic Go)
- [ ] All exported types, functions, and interfaces have GoDoc comments
- [ ] No unused code, dead code, or commented-out blocks
- [ ] Proper error handling (no silent failures, errors are wrapped with context)
- [ ] No panics in production code (except for truly unrecoverable errors)
- [ ] Concurrency is safe (no data races, proper use of mutexes/channels)
- [ ] No hardcoded secrets, credentials, or sensitive data
- [ ] Logging is appropriate (no sensitive data in logs)
- [ ] Unit and integration tests are present for all new features/bugfixes
- [ ] Test coverage is maintained or improved

---

## 3. API Review Checklist
- [ ] API endpoints are documented in Swagger and markdown docs
- [ ] Request/response models are validated and documented
- [ ] Consistent use of HTTP status codes and error formats (RFC7807)
- [ ] Rate limiting, authentication, and authorization are enforced where required
- [ ] No breaking changes to public APIs without versioning and migration plan

---

## 4. Security & Compliance
- [ ] Input validation and sanitization for all user input
- [ ] No SQL injection, XSS, or other common vulnerabilities
- [ ] Sensitive operations require authentication and authorization
- [ ] Compliance with KYC/AML and audit requirements
- [ ] Secrets and keys are managed securely (not in code)

---

## 5. Documentation
- [ ] All new modules, packages, and interfaces are documented
- [ ] Complex algorithms and business logic are explained in comments
- [ ] Operational runbooks are updated if operational behavior changes

---

## 6. Review Process
- Use GitHub/GitLab/Bitbucket pull requests for all changes
- Reviewers should leave clear comments and request changes if needed
- Approvals are required before merging
- Major changes require sign-off from tech lead or architect

---

_Last updated: 2025-06-03_
