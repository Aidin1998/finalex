# gen_proto.ps1: Generate Go gRPC code from proto files for Windows PowerShell

# Ensure Go bin is in PATH
$goBin = "$env:USERPROFILE\go\bin"
if ($env:PATH -notlike "*$goBin*") {
    $env:PATH += ";$goBin"
}

# Install protoc plugins if missing
if (-not (Get-Command protoc-gen-go -ErrorAction SilentlyContinue)) {
    Write-Host "Installing protoc-gen-go..."
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
}
if (-not (Get-Command protoc-gen-go-grpc -ErrorAction SilentlyContinue)) {
    Write-Host "Installing protoc-gen-go-grpc..."
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
}

# Run protoc to generate Go code
$protoFile = "api/marketdata/marketdata.proto"
$goOut = "api/marketdata"

if (-not (Test-Path $protoFile)) {
    Write-Error "Proto file not found: $protoFile"
    exit 1
}

Write-Host "Generating Go code from $protoFile..."
protoc --go_out=$goOut --go-grpc_out=$goOut $protoFile

if ($LASTEXITCODE -eq 0) {
    Write-Host "Proto generation successful."
} else {
    Write-Error "Proto generation failed."
    exit 1
}
