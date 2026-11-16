#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# wiwispr — Automated Local Setup (whisper.cpp + Portuguese Model + Binary)
# Zero cloud dependencies, 100% local & private.
# ==============================================================================

MODEL_NAME="${1:-small}" # Options: tiny, base, small, small-q8_0, medium
INSTALL_BIN_DIR="$HOME/.local/bin"
DATA_DIR="$HOME/.local/share/wiwispr"
MODELS_DIR="$DATA_DIR/models"
BUILD_SRC_DIR="$DATA_DIR/whisper.cpp"
PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

mkdir -p "$INSTALL_BIN_DIR" "$MODELS_DIR"

echo "==> [1/4] Compilando o binário 'wiwispr' em Go..."
cd "$PROJECT_DIR"
go build -ldflags="-s -w" -o "$INSTALL_BIN_DIR/wiwispr" ./cmd/wiwispr
mkdir -p "$PROJECT_DIR/bin"
cp -f "$INSTALL_BIN_DIR/wiwispr" "$PROJECT_DIR/bin/wiwispr" 2>/dev/null || true
echo "    ✓ Binário instalado em: $INSTALL_BIN_DIR/wiwispr"

echo "==> [2/4] Preparando e compilando 'whisper.cpp' otimizado para o seu processador/GPU..."
if [ ! -d "$BUILD_SRC_DIR/.git" ]; then
    rm -rf "$BUILD_SRC_DIR"
    git clone --depth 1 https://github.com/ggml-org/whisper.cpp.git "$BUILD_SRC_DIR"
fi

cd "$BUILD_SRC_DIR"

# If cmake is installed, build with CMake (and Vulkan if glslc + vulkan headers are present).
# Otherwise, compile directly with GCC/G++ using AVX2 / AVX-512 / FMA native optimizations!
if command -v cmake >/dev/null 2>&1; then
    CMAKE_EXTRA_ARGS=""
    if [ -d "/usr/include/vulkan" ] && command -v glslc >/dev/null 2>&1; then
        echo "    -> Detectado Vulkan SDK! Ativando aceleração GPU (-DGGML_VULKAN=1)..."
        CMAKE_EXTRA_ARGS="-DGGML_VULKAN=1"
    fi
    cmake -B build -DCMAKE_BUILD_TYPE=Release $CMAKE_EXTRA_ARGS
    cmake --build build --config Release -j "$(nproc)"
    if [ -f "build/bin/whisper-cli" ]; then
        cp -f "build/bin/whisper-cli" "$INSTALL_BIN_DIR/whisper-cli"
    elif [ -f "build/bin/main" ]; then
        cp -f "build/bin/main" "$INSTALL_BIN_DIR/whisper-cli"
    fi
else
    echo "    -> Compilando whisper-cli diretamente com GCC/G++ (-O3 -march=native -fopenmp)..."
    rm -rf build-direct
    mkdir -p build-direct

    # Generate ggml-version.h required by ggml/src/ggml.c
    cat > build-direct/ggml-version.h << 'EOF'
#pragma once
#define GGML_VERSION "1.7.4"
#define GGML_COMMIT  "local"
EOF

    DEFS='-D_GNU_SOURCE -D_XOPEN_SOURCE=600 -DGGML_SCHED_MAX_COPIES=4 -DGGML_USE_OPENMP -DGGML_USE_CPU -DGGML_USE_LLAMAFILE -DGGML_USE_CPU_REPACK -DWHISPER_VERSION="1.7.4"'
    ARCH_FLAGS="-O3 -march=native -mtune=native -pthread -fopenmp"
    # Note: -Iexamples must precede -Iggml/src/ggml-cpu because both contain a file named common.h!
    INCLUDES="-Ibuild-direct -I. -Iinclude -Isrc -Iggml/include -Iggml/src -Iexamples -Iggml/src/ggml-cpu"

    C_SRCS=(
        ggml/src/ggml.c
        ggml/src/ggml-alloc.c
        ggml/src/ggml-quants.c
        ggml/src/ggml-cpu/ggml-cpu.c
        ggml/src/ggml-cpu/quants.c
        ggml/src/ggml-cpu/arch/x86/quants.c
    )

    CXX_SRCS=(
        ggml/src/ggml.cpp
        ggml/src/ggml-backend.cpp
        ggml/src/ggml-backend-meta.cpp
        ggml/src/ggml-backend-dl.cpp
        ggml/src/ggml-backend-reg.cpp
        ggml/src/ggml-opt.cpp
        ggml/src/ggml-threading.cpp
        ggml/src/gguf.cpp
        ggml/src/ggml-cpu/ggml-cpu.cpp
        ggml/src/ggml-cpu/repack.cpp
        ggml/src/ggml-cpu/hbm.cpp
        ggml/src/ggml-cpu/traits.cpp
        ggml/src/ggml-cpu/amx/amx.cpp
        ggml/src/ggml-cpu/amx/mmq.cpp
        ggml/src/ggml-cpu/tiled/tiled.cpp
        ggml/src/ggml-cpu/tiled/tiled-kernel.cpp
        ggml/src/ggml-cpu/binary-ops.cpp
        ggml/src/ggml-cpu/unary-ops.cpp
        ggml/src/ggml-cpu/vec.cpp
        ggml/src/ggml-cpu/ops.cpp
        ggml/src/ggml-cpu/llamafile/sgemm.cpp
        ggml/src/ggml-cpu/arch/x86/repack.cpp
        ggml/src/ggml-cpu/arch/x86/cpu-feats.cpp
        src/whisper.cpp
        src/aneforge/whisper-aneforge.cpp
        examples/common.cpp
        examples/common-ggml.cpp
        examples/common-whisper.cpp
        examples/grammar-parser.cpp
        examples/cli/cli.cpp
    )

    idx=0
    for src in "${C_SRCS[@]}"; do
        if [ -f "$src" ]; then
            idx=$((idx + 1))
            gcc -std=c11 $ARCH_FLAGS $DEFS $INCLUDES -c "$src" -o "build-direct/c_${idx}.o" &
        fi
    done

    for src in "${CXX_SRCS[@]}"; do
        if [ -f "$src" ]; then
            idx=$((idx + 1))
            g++ -std=c++17 $ARCH_FLAGS $DEFS $INCLUDES -c "$src" -o "build-direct/cxx_${idx}.o" &
        fi
    done

    # Wait for all parallel compilation jobs to complete
    wait

    g++ -std=c++17 $ARCH_FLAGS build-direct/*.o -o "$INSTALL_BIN_DIR/whisper-cli" -lm -lpthread -lgomp -ldl
fi

echo "    ✓ whisper-cli instalado em: $INSTALL_BIN_DIR/whisper-cli"

echo "==> [3/4] Baixando modelo Whisper '$MODEL_NAME' para Português..."
MODEL_FILE="$MODELS_DIR/ggml-${MODEL_NAME}.bin"
if [ -f "$MODEL_FILE" ]; then
    echo "    ✓ Modelo já existe em: $MODEL_FILE"
else
    MODEL_URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-${MODEL_NAME}.bin"
    echo "    -> Fazendo download de $MODEL_URL ..."
    curl -L --progress-bar "$MODEL_URL" -o "${MODEL_FILE}.part"
    mv "${MODEL_FILE}.part" "$MODEL_FILE"
    echo "    ✓ Modelo salvo em: $MODEL_FILE"
fi

echo "==> [4/4] Verificando permissões de digitação (/dev/uinput)..."
if [ -w "/dev/uinput" ]; then
    echo "    ✓ Permissão de escrita em /dev/uinput confirmada!"
else
    echo "    ! Aviso: /dev/uinput não está gravável para o usuário atual."
    echo "      Para liberar sem sudo, execute uma única vez:"
    echo "      sudo setfacl -m u:$USER:rw /dev/uinput"
fi

echo ""
echo "=================================================================="
echo " Instalação do wiwispr concluída com sucesso!"
echo " Binário: $INSTALL_BIN_DIR/wiwispr"
echo " Modelo:  $MODEL_FILE"
echo " Para registrar o atalho SUPER+SHIFT+F no KDE Plasma, execute:"
echo "   ./scripts/register-shortcut.sh"
echo "=================================================================="
