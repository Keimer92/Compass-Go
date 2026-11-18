# Genera la cabecera del asistente NSIS (150x57 BMP) con la marca de Compass Go.
# Uso: powershell -File packaging\make_header.ps1 -Icon assets\icon\CompassGo.ico -Out dist\installer_header.bmp
param(
    [string]$Icon = "assets\icon\CompassGo.ico",
    [string]$Out  = "dist\installer_header.bmp"
)

$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Drawing
# WPF decodifica los frames PNG del .ico con su alfa intacto; GDI+
# (System.Drawing.Icon.ToBitmap) los pinta como ruido.
[void][System.Reflection.Assembly]::LoadWithPartialName("PresentationCore")
[void][System.Reflection.Assembly]::LoadWithPartialName("WindowsBase")

$width  = 150
$height = 57
$iconSize = 48

# Extrae el frame del .ico mas cercano al tamano pedido como Bitmap de GDI+.
function Get-IconBitmap([string]$path, [int]$size) {
    $dec = New-Object System.Windows.Media.Imaging.IconBitmapDecoder(
        (New-Object System.Uri($path)),
        [System.Windows.Media.Imaging.BitmapCreateOptions]::PreservePixelFormat,
        [System.Windows.Media.Imaging.BitmapCacheOption]::OnLoad)

    $frame = @($dec.Frames) |
        Sort-Object -Property @{ Expression = { [math]::Abs($_.PixelWidth - $size) } } |
        Select-Object -First 1

    $enc = New-Object System.Windows.Media.Imaging.PngBitmapEncoder
    $enc.Frames.Add([System.Windows.Media.Imaging.BitmapFrame]::Create($frame))
    $ms = New-Object System.IO.MemoryStream
    $enc.Save($ms)
    $ms.Position = 0

    $tmp = New-Object System.Drawing.Bitmap($ms)
    $bmp = New-Object System.Drawing.Bitmap($tmp)   # copia independiente del stream
    $tmp.Dispose()
    $ms.Dispose()
    return $bmp
}

$dir = Split-Path -Parent $Out
if ($dir -and -not (Test-Path -LiteralPath $dir)) {
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
}

$bmp = New-Object System.Drawing.Bitmap($width, $height, [System.Drawing.Imaging.PixelFormat]::Format24bppRgb)
$g = [System.Drawing.Graphics]::FromImage($bmp)

try {
    $g.SmoothingMode     = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $g.TextRenderingHint = [System.Drawing.Text.TextRenderingHint]::ClearTypeGridFit
    $g.Clear([System.Drawing.Color]::White)

    $textX = 8
    if ($Icon -and (Test-Path -LiteralPath $Icon)) {
        $iconPath = (Resolve-Path -LiteralPath $Icon).Path
        $icoBmp = Get-IconBitmap $iconPath $iconSize
        try {
            $g.InterpolationMode  = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
            $g.PixelOffsetMode    = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
            $g.CompositingQuality = [System.Drawing.Drawing2D.CompositingQuality]::HighQuality
            $g.DrawImage($icoBmp, [System.Drawing.Rectangle]::new(6, 4, $iconSize, $iconSize))
        } finally {
            $icoBmp.Dispose()
        }
        $textX = 62
    }

    $titleFont = New-Object System.Drawing.Font("Segoe UI", 10, [System.Drawing.FontStyle]::Bold, [System.Drawing.GraphicsUnit]::Point)
    $subFont   = New-Object System.Drawing.Font("Segoe UI", 6.5, [System.Drawing.FontStyle]::Regular, [System.Drawing.GraphicsUnit]::Point)
    $titleBrush = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(32, 32, 32))
    $subBrush   = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(110, 110, 110))

    try {
        $g.DrawString("Compass Go", $titleFont, $titleBrush, [single]$textX, [single]14)
        $g.DrawString("GUI para proyectos", $subFont, $subBrush, [single]($textX + 1), [single]32)
    } finally {
        $titleFont.Dispose()
        $subFont.Dispose()
        $titleBrush.Dispose()
        $subBrush.Dispose()
    }

    $bmp.Save($Out, [System.Drawing.Imaging.ImageFormat]::Bmp)
} finally {
    $g.Dispose()
    $bmp.Dispose()
}

Write-Output "Cabecera generada: $Out"
