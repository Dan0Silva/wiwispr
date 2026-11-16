# 🎙️ wiwispr

**`wiwispr`** é uma ferramenta ultraleve, 100% local e privada de ditado por voz para Linux (projetada especialmente para **Fedora / KDE Plasma Wayland**).

Ao pressionar um atalho global (ex: `SUPER + S`), ela exibe um indicador flutuante minimalista com animação de ondas sonoras — **sem roubar o foco da janela atual** — escuta o que você diz em Português, transcreve localmente via **`whisper.cpp`** e digita tecla por tecla no campo de texto selecionado (terminal, navegador, IDE, etc.).

---

## ✨ Principais Características

- **Zero Nuvem / 100% Privado**: Nenhum byte de áudio sai do seu computador. Tudo roda localmente via `whisper.cpp`.
- **Zero Consumo em Segundo Plano**: O programa não precisa ficar rodando como *daemon* gastando memória RAM. Ele inicia em menos de 15ms ao apertar o atalho, faz o trabalho (usando `/dev/shm` direto na RAM para não desgastar seu SSD) e fecha completamente ao terminar.
- **Parada Inteligente Dupla (Silêncio ou Manual)**:
  - **Automática (VAD)**: Analisa o volume RMS da sua voz em tempo real (a cada 50ms) e para automaticamente após **~2 segundos de silêncio** depois que você termina de falar.
  - **Manual (Toggle via UNIX Socket)**: Se não quiser esperar os 2 segundos de silêncio, basta apertar `SUPER + S` novamente e ele para a gravação na hora e já digita o texto.
- **Overlay Animado (HUD) Invisível para o Foco**:
  - Fica sempre no topo de qualquer janela (`Always on Top` / `override-redirect`).
  - **Não rouba o foco do teclado** (`accept_focus=False`, `focus_on_map=False`) e é transparente a cliques de mouse (*click-through*), garantindo que o cursor continue piscando exatamente onde você estava digitando.
  - Sem barra de título, sem bordas do sistema e sem botões de fechar/minimizar/maximizar.
  - Ondas sonoras animadas a **60 FPS** reagindo à sua voz em tempo real.
- **Digitação Nativa Tecla por Tecla (`/dev/uinput`)**:
  - Cria um teclado virtual no kernel Linux sem depender de ferramentas externas (`xdotool`/`ydotool`), com suporte completo a **acentos e cedilha do Português (`á, é, í, ó, ú, ã, õ, â, ê, ô, à, ç`)** tanto em teclados **US-International (`us-intl`)** quanto **ABNT2 (`abnt2`)**.

---

## 🏗️ Arquitetura do Projeto

```text
wiwispr/
├── cmd/
│   └── wiwispr/
│       └── main.go              # CLI principal, ciclo de vida e modos de diagnóstico
├── internal/
│   ├── audio/
│   │   ├── recorder.go          # Captura de áudio (arecord/pw-record) + Detector de Silêncio (VAD RMS)
│   │   └── recorder_test.go     # Testes unitários do detector de voz/silêncio e geração WAV
│   ├── config/
│   │   └── config.go            # Configurações padrão e leitura de variáveis de ambiente
│   ├── ipc/
│   │   ├── socket.go            # Controle de Toggle via UNIX Socket (/run/user/$UID/wiwispr.sock)
│   │   └── socket_test.go       # Testes de comunicação entre instâncias
│   ├── transcriber/
│   │   ├── whisper.go           # Execução do whisper-cli e limpeza de alucinações de silêncio
│   │   └── whisper_test.go      # Testes de limpeza de texto
│   ├── typer/
│   │   ├── typer.go             # Teclado virtual Linux (/dev/uinput) com suporte PT-BR (US-Intl e ABNT2)
│   │   └── typer_test.go        # Testes de conversão de caracteres acentuados para keycodes
│   └── ui/
│       ├── overlay.go           # Gerenciador Go do processo de interface visual (embed)
│       └── overlay.py           # HUD em GTK3/Cairo a 60 FPS (sem bordas, sempre no topo, sem foco)
├── scripts/
│   ├── setup.sh                 # Compila wiwispr, compila whisper.cpp otimizado e baixa o modelo PT-BR
│   └── register-shortcut.sh     # Configura o atalho SUPER + S no KDE Plasma 6
├── Makefile
└── go.mod
```

---

## 🚀 Instalação e Configuração Rápida

### 1. Executar o Script de Setup Automático

O script `scripts/setup.sh` faz tudo automaticamente:
1. Compila o binário `wiwispr` em Go e instala em `~/.local/bin/wiwispr`.
2. Baixa e compila o `whisper.cpp` otimizado com instruções nativas do seu processador (`AVX-512` / `AVX2` / `FMA`) ou `Vulkan` (se o SDK estiver instalado).
3. Baixa o modelo oficial `ggml-small.bin` (excelente precisão e pontuação em Português) em `~/.local/share/wiwispr/models/`.
4. Verifica as permissões do `/dev/uinput`.

```bash
chmod +x scripts/setup.sh scripts/register-shortcut.sh
./scripts/setup.sh
```

> **Dica de Modelos:** Caso prefira um modelo ainda mais leve (porém com menos precisão de pontuação que o `small`), você pode passar `base` como argumento:
> ```bash
> ./scripts/setup.sh base
> ```

---

### 2. Configurar o Atalho `SUPER + S` no KDE Plasma

Execute o script abaixo para registrar o atalho global no KDE:

```bash
./scripts/register-shortcut.sh
```

Ou, se preferir configurar visualmente pelo painel do KDE Plasma 6:
1. Abra as **Configurações do Sistema** (`System Settings`) → **Teclado** → **Atalhos** (`Shortcuts`).
2. Clique em **Adicionar Novo** (`Add New`) → **Comando ou Script** (`Command`).
3. Em **Nome**, digite: `wiwispr`
4. Em **Comando**, coloque o caminho completo:
   ```text
   /home/v0rtx/.local/bin/wiwispr
   ```
5. Clique no botão de atalho, pressione **`SUPER + S`** (`Meta + S`) e clique em **Aplicar**.

---

## 🛠️ Opções de Linha de Comando e Personalização

Você pode ajustar o comportamento do `wiwispr` via flags no comando do atalho ou variáveis de ambiente:

| Flag CLI | Variável de Ambiente | Padrão | Descrição |
| :--- | :--- | :--- | :--- |
| `-silence` | `WIWISPR_SILENCE_SEC` | `2.0` | Segundos de silêncio após a fala para parar automaticamente. |
| `-threshold` | `WIWISPR_THRESHOLD` | `0.018` | Sensibilidade do microfone para detectar voz (RMS `0.005` a `0.15`). |
| `-delay` | `WIWISPR_TYPING_DELAY_MS` | `12` | Atraso (em milissegundos) entre cada tecla digitada (efeito máquina de escrever). |
| `-layout` | `WIWISPR_KB_LAYOUT` | `us-intl` | Layout do seu teclado no sistema (`us-intl` para US Internacional com dead keys, ou `abnt2` para teclado brasileiro com `ç`). |
| `-lang` | `WIWISPR_LANG` | `pt` | Idioma da transcrição (`pt`, `en`, `es`, etc.). |
| `-model` | `WIWISPR_MODEL` | `.../ggml-small.bin` | Caminho para o arquivo `.bin` do modelo Whisper. |
| `-no-overlay` | `WIWISPR_NO_OVERLAY=1` | `false` | Desativa o ícone flutuante caso queira execução 100% invisível. |

### Exemplos de Uso / Diagnóstico

- **Testar apenas a animação do ícone flutuante por 3 segundos:**
  ```bash
  ./bin/wiwispr -test-overlay 3s
  ```
- **Testar a digitação tecla por tecla com acentos em Português:**
  ```bash
  ./bin/wiwispr -type-text "Olá! Testando acentuação em português: ação, coração, você."
  ```
- **Usar layout de teclado ABNT2 e parar com 1.5s de silêncio:**
  ```bash
  wiwispr -layout abnt2 -silence 1.5
  ```

---

## 🧪 Rodando os Testes Automatizados

Para executar a suíte de testes unitários (VAD/detecção de silêncio, socket IPC, limpeza de transcrição e mapeamento de teclas PT-BR):

```bash
go test -v ./...
```

---

## ❓ Solução de Problemas (Troubleshooting)

1. **Os acentos (`ã`, `ç`, `é`) saem trocados ao digitar:**
   - Isso depende do layout de teclado ativo no seu KDE Plasma.
   - Se o seu teclado está configurado como **Inglês Internacional** (`us` com variante `alt-intl` ou `intl`), use o padrão `-layout us-intl`.
   - Se o seu teclado está configurado como **Português (Brasil ABNT2)**, passe `-layout abnt2` (ou defina `WIWISPR_KB_LAYOUT=abnt2`).
2. **A gravação não para sozinha depois que termino de falar (microfone com muito ruído de fundo):**
   - Aumente levemente o limiar de voz com `-threshold 0.035` (ou lembre-se que você sempre pode apertar `SUPER + S` novamente para parar na hora!).
3. **Permissão negada em `/dev/uinput` após reiniciar:**
   - No Fedora com sessão gráfica ativa (systemd-logind), seu usuário já recebe permissão ACL automática em `/dev/uinput`. Caso algum dia falhe, o `wiwispr` copia o texto automaticamente para o seu Clipboard (`wl-copy`) para você não perder o que falou, ou você pode liberar com:
     ```bash
     sudo setfacl -m u:$USER:rw /dev/uinput
     ```
