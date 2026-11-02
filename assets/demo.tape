# Demo del README. Se genera con: vhs assets/demo.tape
#
# VHS no graba una sesión: ejecuta este guion. El mismo archivo da siempre el
# mismo GIF, así que se puede versionar y regenerar cuando cambie la salida.
#
# Los comandos son cortos a propósito: con todas las flags en la línea, el texto
# se sale por la derecha y en el vídeo sólo se lee la mitad. Se usa un préstamo
# guardado, que además es como se usa la herramienta de verdad.

Output assets/demo.gif

Set Shell "powershell"
Set FontSize 16
Set FontFamily "Cascadia Mono"
Set Width 1100
Set Height 560
Set Padding 24
Set MarginFill "#1e1e2e"
Set BorderRadius 12
Set Theme "Catppuccin Mocha"
Set TypingSpeed 35ms

# Preparación: el préstamo se da de alta una vez y no se muestra en el vídeo.
# Si ya existe, el error se ignora a propósito.
Hide
Type "hipoteca loan remove casa 2>$null | Out-Null"
Enter
Type "hipoteca loan add casa --name Casa --type variable --start-date 2021-06-01 --principal 180000 --months 360 --payment-day 1 --initial-rate 1.25 --initial-period-months 12 --index euribor-1y --spread 0.89 --revision-every-months 12 --reference-offset-months 2 --floor none --rate-rounding none --fee-pct 0 2>$null | Out-Null"
Enter
Type "Clear-Host"
Enter
Show

# --- 1. De dónde salen los datos ---
Type "hipoteca index list --pretty"
Enter
Sleep 3s

# --- 2. La revisión: el cálculo que nadie hace a mano ---
Hide
Type "Clear-Host"
Enter
Show

Type "hipoteca revision casa --date 2022-06 --pretty"
Enter
Sleep 5s

# --- 3. Amortizar 10.000 €, los dos modos lado a lado ---
Hide
Type "Clear-Host"
Enter
Show

Type "hipoteca amortize casa --amount 10000 --date 60 --mode compare --pretty"
Enter
Sleep 5s
