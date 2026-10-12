param([switch]$SkipBuild)
$ErrorActionPreference = 'Stop'
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
Push-Location -LiteralPath $repo
try {
    if (-not $SkipBuild) {
        & wails build -platform windows/amd64 -m -nosyncgomod -trimpath -webview2 error
        if ($LASTEXITCODE -ne 0) { throw 'Portable desktop build failed.' }
        # Wails code generation emits whitespace-only lines; normalize output.
        $models = Join-Path $repo 'frontend\wailsjs\go\models.ts'
        $generated = [IO.File]::ReadAllText($models) -replace '(?m)[\t ]+\r?$', ''
        [IO.File]::WriteAllText($models, ($generated.TrimEnd() + "`n"), [Text.UTF8Encoding]::new($false))
    }
    $binary = Join-Path $repo 'build\bin\BrowserRuntime.exe'
    if (-not (Test-Path -LiteralPath $binary -PathType Leaf)) { throw 'Portable binary is missing.' }
    $output = Join-Path $repo 'build\bin\BrowserRuntime-v0.1.0-Windows-x64.zip'
    # Explicit allowlist: never recursively package the repository or live data.
    $stream = [IO.File]::Open($output, [IO.FileMode]::Create)
    try {
        $zip = [IO.Compression.ZipArchive]::new($stream, [IO.Compression.ZipArchiveMode]::Create, $true)
        try {
            [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, $binary, 'BrowserRuntime.exe', [IO.Compression.CompressionLevel]::Optimal) | Out-Null
            [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, (Join-Path $repo 'docs\PORTABLE_README.md'), 'README.md', [IO.Compression.CompressionLevel]::Optimal) | Out-Null
        } finally { $zip.Dispose() }
    } finally { $stream.Dispose() }
    [pscustomobject]@{ ZIP=$output; Bytes=(Get-Item -LiteralPath $output).Length; SHA256=(Get-FileHash -LiteralPath $output -Algorithm SHA256).Hash }
} finally { Pop-Location }
