# Install the latest ccxray release on Windows.
#   irm https://raw.githubusercontent.com/dontfeedthecode/cc-xray/main/install.ps1 | iex
#
# $env:CCXRAY_VERSION = 'vX.Y.Z'      install a specific release
# $env:CCXRAY_INSTALL_DIR = '<dir>'   install somewhere other than ~\.local\bin
#
# Everything runs inside a script block and fails with throw, never exit:
# piped into iex, exit would close the shell it was run from.
& {
    $ErrorActionPreference = 'Stop'
    # Invoke-WebRequest's progress bar slows Windows PowerShell 5.1 to a crawl.
    $ProgressPreference = 'SilentlyContinue'

    $repo = 'dontfeedthecode/cc-xray'
    $bin = 'ccxray.exe'

    if ($PSVersionTable.PSEdition -eq 'Core' -and -not $IsWindows) {
        throw "ccxray: this installer is for Windows. On macOS and Linux run:`n  curl -fsSL https://raw.githubusercontent.com/$repo/main/install.sh | sh"
    }

    # Windows PowerShell 5.1 can default to TLS 1.0, which GitHub refuses.
    [Net.ServicePointManager]::SecurityProtocol =
        [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    # A 32-bit PowerShell on 64-bit Windows sees x86 here and the real
    # architecture in PROCESSOR_ARCHITEW6432.
    $cpu = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    $arch = switch ($cpu) {
        'AMD64' { 'amd64' }
        'ARM64' { 'arm64' }
        default { throw "ccxray: unsupported architecture: $cpu" }
    }

    $tag = $env:CCXRAY_VERSION
    if (-not $tag) {
        try {
            $tag = (Invoke-RestMethod -UseBasicParsing "https://api.github.com/repos/$repo/releases/latest").tag_name
        } catch {}
    }
    if (-not $tag) {
        throw "ccxray: could not determine the latest release.`n  Set `$env:CCXRAY_VERSION = 'vX.Y.Z', or download from https://github.com/$repo/releases"
    }

    $ver = $tag -replace '^v', ''
    $archive = "ccxray_${ver}_windows_${arch}.zip"
    $base = "https://github.com/$repo/releases/download/$tag"
    $dest = if ($env:CCXRAY_INSTALL_DIR) { $env:CCXRAY_INSTALL_DIR } else { Join-Path $HOME '.local\bin' }

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ('ccxray-' + [guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "ccxray: downloading $tag (windows/$arch)"
        $zip = Join-Path $tmp $archive
        $sums = Join-Path $tmp 'checksums.txt'
        try { Invoke-WebRequest -UseBasicParsing "$base/$archive" -OutFile $zip }
        catch { throw "ccxray: download failed: $base/$archive" }
        try { Invoke-WebRequest -UseBasicParsing "$base/checksums.txt" -OutFile $sums }
        catch { throw "ccxray: download failed: $base/checksums.txt" }

        $want = ''
        foreach ($line in Get-Content $sums) {
            $f = -split $line
            if ($f.Count -ge 2 -and $f[1] -eq $archive) { $want = $f[0] }
        }
        $got = (Get-FileHash -Algorithm SHA256 $zip).Hash
        # -ne ignores case: checksums.txt is lower-case hex, Get-FileHash upper.
        if (-not $want -or $want -ne $got) { throw "ccxray: checksum mismatch for $archive" }

        Expand-Archive -Path $zip -DestinationPath $tmp -Force
        $src = Join-Path $tmp $bin
        if (-not (Test-Path $src)) { throw "ccxray: $bin not found in $archive" }

        New-Item -ItemType Directory -Force -Path $dest | Out-Null
        # Windows will not overwrite an exe that is running.
        try { Copy-Item -Force $src (Join-Path $dest $bin) }
        catch { throw "ccxray: cannot write $bin to $dest. If ccxray is running, quit it and run this again; otherwise set `$env:CCXRAY_INSTALL_DIR." }
    } finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }
    Write-Host "ccxray: installed $tag to $(Join-Path $dest $bin)"

    # Compare PATH entries as Windows does: expanded, without a trailing
    # slash, ignoring case (-contains already does).
    $entries = { param($p) $p -split ';' | Where-Object { $_ } |
        ForEach-Object { [Environment]::ExpandEnvironmentVariables($_).TrimEnd('\') } }
    $mine = $dest.TrimEnd('\')

    # Edit the registry value directly, not through SetEnvironmentVariable,
    # so entries written as %USERPROFILE%\... stay unexpanded.
    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
    try {
        $raw = $key.GetValue('Path', '', 'DoNotExpandEnvironmentNames')
        if ((& $entries $raw) -notcontains $mine) {
            $key.SetValue('Path', ($raw.TrimEnd(';') + ';' + $dest).TrimStart(';'), 'ExpandString')
            # Setting any user variable broadcasts the change, so terminals
            # opened from now on see the new PATH.
            [Environment]::SetEnvironmentVariable('CCXRAY_INSTALL', '1', 'User')
            [Environment]::SetEnvironmentVariable('CCXRAY_INSTALL', $null, 'User')
            Write-Host "ccxray: added $dest to your user PATH; terminals opened from now on will find ccxray."
        }
    } finally {
        $key.Close()
    }
    if ((& $entries $env:Path) -notcontains $mine) { $env:Path = "$env:Path;$dest" }

    Write-Host "ccxray: run 'ccxray' in a second terminal, in the directory Claude Code is working in."
}
