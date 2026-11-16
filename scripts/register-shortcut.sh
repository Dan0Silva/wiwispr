#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# Registra o atalho global Meta+Shift+F (SUPER + SHIFT + F) no KDE Plasma 6
# ==============================================================================

BIN_PATH="$HOME/.local/bin/wiwispr"
if [ ! -x "$BIN_PATH" ]; then
    PROJECT_BIN="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/bin/wiwispr"
    if [ -x "$PROJECT_BIN" ]; then
        mkdir -p "$HOME/.local/bin"
        cp -f "$PROJECT_BIN" "$BIN_PATH"
    fi
fi

APPS_DIR="$HOME/.local/share/applications"
DESKTOP_FILE="$APPS_DIR/wiwispr.desktop"
mkdir -p "$APPS_DIR"

cat > "$DESKTOP_FILE" <<EOF
[Desktop Entry]
Type=Application
Name=wiwispr Voice Dictation
Comment=Transcreve voz localmente e digita no campo em foco
Exec=$BIN_PATH
Icon=audio-input-microphone
Terminal=false
Categories=Utility;Accessibility;
StartupNotify=false
X-KDE-Shortcuts=Meta+S
EOF

chmod +x "$DESKTOP_FILE"

if command -v kwriteconfig6 >/dev/null 2>&1; then
    kwriteconfig6 --file kglobalshortcutsrc --group "services" --group "wiwispr.desktop" --key "_launch" "Meta+S"
    echo "✓ Atalho 'Meta+Shift+F' registrado em ~/.config/kglobalshortcutsrc"
fi

if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database "$APPS_DIR" >/dev/null 2>&1 || true
fi

# Solicita ao KGlobalAccel recarregar configurações se disponível via gdbus
if command -v gdbus >/dev/null 2>&1; then
    gdbus call --session --dest org.kde.kded6 --object-path /kded --method org.kde.kded6.reconfigure >/dev/null 2>&1 || true
fi

echo "✓ Arquivo de atalho criado em: $DESKTOP_FILE"
echo ""
echo "Dica: Se o KDE Plasma não ativar o atalho imediatamente, você também pode abrir:"
echo "  Configurações do Sistema -> Teclado -> Atalhos -> Adicionar Novo -> Comando"
echo "  Comando: $BIN_PATH"
echo "  Atalho:  Meta + S"
