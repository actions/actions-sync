# Determine file to download based on current OS
if($IsLinux) {
   $file_postfix = "linux_amd64"
} elseif ($IsWindows) {
   $file_postfix = "windows_amd64"
} elseif ($IsMacOS) {
    $file_postfix = "darwin_amd64"
}

# Download release to test
curl -OL "https://github.com/actions/actions-sync/releases/download/v$Env:RELEASEDATE/gh_$Env:RELEASEDATE`_$file_postfix.tar.gz"

# extract
tar -xvzf "gh_$Env:RELEASEDATE`_$file_postfix.tar.gz"

# prepare cache directory
mkdir -p cache