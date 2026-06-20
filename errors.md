# Plan: Qualitative Error Review and Improvement

This document outlines the plan to systematically review and improve error handling across the `go-anansi` codebase, focusing on the quality and utility of error messages and propagation.

---

## Goal

The primary goal is to ensure that all errors returned by the Anansi system are:

*   **Contextual:** Provide information relevant to *where* and *when* the error occurred within the application's logic.
*   **Helpful:** Guide developers and, where appropriate, end-users towards understanding the problem and potential solutions or next steps.
*   **Non-redundant:** Avoid repeating information unnecessarily within an error chain or message. Each piece of information should add value.
*   **Specific:** Pinpoint the exact nature of the problem, avoiding vague or generic messages.
*   **Detailed:** Offer sufficient information for debugging, logging, and informed decision-making without overwhelming the consumer.

---

## Guiding Principles for Error Quality

These principles build upon the foundation of `core/common.SystemError` and its structured approach.

1.  **Consistent `SystemError` Usage:** All application-level errors *must* be `*common.SystemError` instances. This ensures a standardized structure for error codes, messages, and contextual information.
2.  **Meaningful Error Codes:** `SystemError.Code` should be a machine-readable, specific identifier (e.g., `ERR_EPHEMERAL_NOT_TRANSACTION`). Avoid generic codes like `INTERNAL_ERROR` unless truly no more specific code applies.
3.  **Clear `SystemError.Message`:** `SystemError.Message` should be a concise, human-readable summary of the error. It should be suitable for direct display or logging.
4.  **Contextual `SystemError.Path` and `SystemError.Operation`:**
    *   `Path`: Use `SystemError.WithPath()` to indicate the specific data path or field related to the error (e.g., "user.email", "items[0].price").
    *   `Operation`: Use `SystemError.WithOperation()` to describe the high-level action being performed when the error occurred (e.g., "repository.Insert", "document.Validate").
5.  **Proper Error Wrapping (`%w` and `WithCause`):**
    *   Always wrap underlying errors using `fmt.Errorf("additional context: %w", originalError)` or `SystemError.WithCause(originalError)`. This preserves the original error's type and allows for inspection using `errors.Is()` and `errors.As()`.
    *   Avoid `fmt.Errorf("...%v", originalError)` as it discards type information.
    *   Avoid creating new errors with messages that simply duplicate the underlying error's message. Add *new* context.
6.  **Abstraction and Information Hiding:**
    *   Platform-specific or low-level errors (e.g., database driver errors) should be caught and wrapped by higher-level, abstract `SystemError` instances.
    *   The `SystemError.Message` and `Code` should reflect the application-level problem, while the `Cause` retains the low-level detail for debugging.
    *   Avoid leaking implementation details (e.g., SQL error messages) directly into top-level `SystemError.Message` unless absolutely necessary and sanitized.
7.  **Aggregating Multiple Issues:** For scenarios like validation, use `SystemError.WithIssue()` or `SystemError.WithIssues()` to collect multiple distinct problems into a single `SystemError`, providing a comprehensive error report.

---

## Review Methodology

For each package in the checklist below, we will perform the following steps:

1.  **Initial Scan:** Read all `.go` files in the package.
2.  **Identify Error Creation Points:** Locate every instance where an error is returned or created:
    *   `return errors.New(...)`
    *   `return fmt.Errorf(...)`
    *   `return common.NewSystemError(...)`
    *   `return common.SystemErrorFrom(...)`
    *   Returning pre-defined error variables (e.g., `ephemeral.ErrNotTransaction`).
3.  **Analyze Error Quality:** For each identified error, evaluate it against the "Goal" and "Guiding Principles" above:
    *   Is it a `SystemError`? If not, convert it.
    *   Is the `Code` specific and meaningful?
    *   Is the `Message` clear, concise, and helpful?
    *   Is `Path` or `Operation` relevant and used where appropriate?
    *   Is the error properly wrapped with `WithCause` or `%w` if it's an underlying error?
    *   Does it leak unnecessary implementation details?
    *   Could multiple issues be aggregated?
4.  **Propose Improvements:** Based on the analysis, suggest concrete changes to the error creation or propagation logic.
5.  **Implement and Verify:** Apply the changes, compile the project, and run relevant tests.
6.  **Document:** Document each error in the following manner in ./system-errors.md.
```markdown

## Example Error Message (ERR_ERROR_CODE_HERE)
This error is caused by x and y. It results in z and a.
Or whatever other important information about the error we need to know.

**Error Chain**: sqlite -> query.native -> persistence.collection

**Packages**: packages, that, throw, this/error, in/order
**Methods**: methods.that, can.throw, this.error
**Severity**: how severe is the error?
```

---

## Master Checklist: Packages for Qualitative Error Review

We will proceed through this list one package at a time.

### Core Packages
- [x] `core/common`
- [x] `core/data`
- [x] `core/ephemeral`
- [x] `core/events`
- [x] `core/persistence/base`
- [x] `core/persistence/collection`
- [x] `core/persistence/persistence`
- [x] `core/persistence/registry`
- [x] `core/persistence/transaction`
- [ ] `core/query`
- [x] `core/query/native`
- [x] `core/query/parser`
- [x] `core/schema`
- [ ] `core/schema/codegen`

### SQLite Packages
- [ ] `sqlite/executor`
- [ ] `sqlite/query`

---
