# PowerShell script to migrate error responses
$filePath = "internal/trading/handlers/trading_handlers.go"

# Read the file content
$content = Get-Content $filePath -Raw

# Replace import
$content = $content -replace '"github\.com/Aidin1998/finalex/common/apiutil"', '"github.com/Aidin1998/finalex/api/responses"'

# Replace BadRequest errors
$content = $content -replace 'c\.JSON\(http\.StatusBadRequest, apiutil\.ErrorResponse\{\s*Error:\s*"[^"]*",\s*Message:\s*"([^"]*)"[^}]*\}\)', 'responses.BadRequest(c, "$1")'

# Replace Unauthorized errors
$content = $content -replace 'c\.JSON\(http\.StatusUnauthorized, apiutil\.ErrorResponse\{\s*Error:\s*"[^"]*",\s*Message:\s*"([^"]*)"[^}]*\}\)', 'responses.Unauthorized(c, "$1")'

# Replace Forbidden errors
$content = $content -replace 'c\.JSON\(http\.StatusForbidden, apiutil\.ErrorResponse\{\s*Error:\s*"[^"]*",\s*Message:\s*"([^"]*)"[^}]*\}\)', 'responses.Forbidden(c, "$1")'

# Replace NotFound errors
$content = $content -replace 'c\.JSON\(http\.StatusNotFound, apiutil\.ErrorResponse\{\s*Error:\s*"[^"]*",\s*Message:\s*"([^"]*)"[^}]*\}\)', 'responses.NotFound(c, "$1")'

# Replace InternalServerError errors
$content = $content -replace 'c\.JSON\(http\.StatusInternalServerError, apiutil\.ErrorResponse\{\s*Error:\s*"[^"]*",\s*Message:\s*"([^"]*)"[^}]*\}\)', 'responses.InternalServerError(c, "$1")'

# Write back to file
$content | Set-Content $filePath

Write-Host "Migration completed for $filePath"
