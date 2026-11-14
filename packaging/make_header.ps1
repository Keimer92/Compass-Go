# Genera la cabecera del asistente NSIS (150x57 BMP) con la marca de Compass Go.
# Uso: powershell -File packaging\make_header.ps1 -Icon assets\icon\Icon.ico -Out dist\installer_header.bmp
param(
    [string]$Icon = "assets\icon\Icon.ico",
    [string]$Out  = "dist\installer_header.bmp"
)

$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Drawing

$width  = 150
$height = 57

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
        $appIcon = [System.Drawing.Icon]::new($iconPath, 48, 48)
        $g.DrawIcon($appIcon, [System.Drawing.Rectangle]::new(6, 4, 48, 48))
        $appIcon.Dispose()
        $textX = 62
    }

    $titleFont = New-Object System.Drawing.Font("Segoe UI", 10, [System.Drawing.FontStyle]::Bold, [System.Drawing.GraphicsUnit]::Point)
    $subFont   = New-Object System.Drawing.Font("Segoe UI", 6.5, [System.Drawing.FontStyle]::Regular, [System.Drawing.GraphicsUnit]::Point)
    $titleBrush = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(32, 32, 32))
    $subBrush   = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(110, 110, 110))

    try {
        $g.DrawString("Compass Go", $titleFont, $titleBrush, [single]$textX, [single]14)
        $g.DrawString("TUI para proyectos", $subFont, $subBrush, [single]($textX + 1), [single]32)
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
