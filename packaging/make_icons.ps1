# Genera un .ico multi-tamano (16..256) a partir de un .png.
# Cada tamano se guarda como PNG dentro del contenedor ICO (soportado por
# Windows Vista+), lo que conserva la calidad y el canal alfa.
#
# Uso:
#   powershell -File packaging\make_icons.ps1 -Source "assets\icon\Compass Go.png" -Out "assets\icon\CompassGo.ico"

param(
    [Parameter(Mandatory = $true)][string]$Source,
    [Parameter(Mandatory = $true)][string]$Out,
    [int[]]$Sizes = @(16, 24, 32, 48, 64, 128, 256)
)

Add-Type -AssemblyName System.Drawing

if (-not (Test-Path -LiteralPath $Source)) {
    Write-Error "No existe el PNG: $Source"
    exit 1
}

$src = [System.Drawing.Image]::FromFile((Resolve-Path -LiteralPath $Source).Path)
try {
    $pngs = @()
    foreach ($size in $Sizes) {
        $bmp = New-Object System.Drawing.Bitmap($size, $size, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
        $g = [System.Drawing.Graphics]::FromImage($bmp)
        $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
        $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::HighQuality
        $g.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
        $g.Clear([System.Drawing.Color]::Transparent)
        $g.DrawImage($src, 0, 0, $size, $size)
        $g.Dispose()

        $ms = New-Object System.IO.MemoryStream
        $bmp.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
        $bmp.Dispose()
        $pngs += , @{ Size = $size; Bytes = $ms.ToArray() }
        $ms.Dispose()
    }

    $outPath = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Out)
    $dir = [System.IO.Path]::GetDirectoryName($outPath)
    if ($dir -and -not (Test-Path -LiteralPath $dir)) {
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
    }

    $fs = [System.IO.File]::Create($outPath)
    $bw = New-Object System.IO.BinaryWriter($fs)
    try {
        # ICONDIR
        $bw.Write([UInt16]0)              # reserved
        $bw.Write([UInt16]1)              # type = icon
        $bw.Write([UInt16]$pngs.Count)    # count

        $offset = 6 + (16 * $pngs.Count)
        foreach ($p in $pngs) {
            $dim = if ($p.Size -ge 256) { 0 } else { $p.Size }
            $bw.Write([Byte]$dim)         # width
            $bw.Write([Byte]$dim)         # height
            $bw.Write([Byte]0)            # color count
            $bw.Write([Byte]0)            # reserved
            $bw.Write([UInt16]1)          # planes
            $bw.Write([UInt16]32)         # bit count
            $bw.Write([UInt32]$p.Bytes.Length)
            $bw.Write([UInt32]$offset)
            $offset += $p.Bytes.Length
        }
        foreach ($p in $pngs) {
            $bw.Write($p.Bytes)
        }
    } finally {
        $bw.Dispose()
        $fs.Dispose()
    }
    Write-Host "[OK] $Out"
} finally {
    $src.Dispose()
}
