param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$InitArguments
)

$ProjectRoot = Split-Path -Parent $PSScriptRoot
& go run "$ProjectRoot/tools/init" --root "$ProjectRoot" @InitArguments
exit $LASTEXITCODE
