# API & WebSocket Error Response Guidelines

## Standard Error Response Structure
All API and WebSocket error responses must use the following JSON structure:

```json
{
  "error": "error_code",                // Short, machine-readable error code (snake_case)
  "message": "Clear human-readable explanation", // Concise, actionable explanation for the user
  "details": "Specific technical details (optional)" // (Optional) Additional context for developers
}
```

### Example
```json
{
  "error": "invalid_quantity",
  "message": "Invalid request: 'quantity' parameter must be a positive decimal with up to 8 decimal places.",
  "details": "Received quantity: -5.00000001"
}
```

## Error Code Conventions
- Use snake_case for error codes (e.g., `invalid_request`, `unauthorized`, `insufficient_balance`).
- Codes should be unique and map to a specific error scenario.
- Maintain a central registry of error codes in this document.

## Error Message Writing Guidelines
- State the issue clearly: What went wrong?
- State the cause: Why did it happen?
- Suggest corrective action: What should the user/developer do?
- Avoid generic messages like "Invalid Request" or "Error occurred".
- Use parameter names and expected formats/types in the message.

### Examples
- **Original:** `{"error": "invalid_request", "message": "Invalid Request"}`
- **Improved:** `{"error": "invalid_quantity", "message": "Invalid request: 'quantity' parameter must be a positive decimal with up to 8 decimal places.", "details": "Received quantity: -5.00000001"}`

- **Original:** `{"error": "unauthorized", "message": "Unauthorized"}`
- **Improved:** `{"error": "unauthorized", "message": "Authentication failed: missing or invalid bearer token.", "details": "No Authorization header present."}`

## Recommended Client-Side Handling
- Always check the `error` field for programmatic handling.
- Display the `message` field to users.
- Log the `details` field for debugging (do not show to end users).
- Implement retry/backoff logic for transient errors (e.g., `rate_limit_exceeded`).
- For validation errors, highlight the specific field(s) in the UI.

## Error Code Registry (Examples)
| Error Code                | Description                                      |
|--------------------------|--------------------------------------------------|
| invalid_request          | Generic invalid request (avoid, use specific)    |
| invalid_quantity         | Quantity parameter invalid                       |
| missing_parameter        | Required parameter missing                       |
| unauthorized             | Authentication failed                           |
| forbidden                | Insufficient permissions                        |
| not_found                | Resource not found                              |
| rate_limit_exceeded      | Too many requests                               |
| insufficient_balance     | Not enough funds for operation                  |
| db_error                 | Database error                                  |
| internal_error           | Unexpected server error                         |
| validation_failed        | Input validation failed                         |
| websocket_protocol_error | WebSocket message format/protocol error          |
| aml_blocked              | Blocked by AML/compliance                       |

## Developer Guidelines
- Use the standard structure for all error responses.
- Always provide a specific error code and a clear, actionable message.
- Add technical details in the `details` field when helpful for debugging.
- Update this document with new error codes as they are introduced.
- Review and refactor legacy error messages for clarity and consistency.

## See Also
- `common/apiutil/gin_errors.go` for reusable error helpers.
- `internal/server/server.go` for endpoint implementations.
- This document should be referenced in code reviews for all error handling changes.
