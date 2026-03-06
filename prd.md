# PRD — Local Voice Dictation Tool

## 1. Product Overview

The product is a **local voice dictation utility for Linux** that allows a user to quickly convert spoken input into cleaned text using local AI models.

The user activates dictation using a global hotkey. Speech is recorded, transcribed locally, cleaned using a local LLM, and inserted into the currently active application.

The system runs entirely **locally**, with **no cloud dependency**.

---

# 2. Goals

Primary goals:

1. **Fast voice-to-text workflow**
2. **Fully local processing**
3. **Minimal UI**
4. **Works in any application**
5. **Single hotkey workflow**

The product should feel **instant and unobtrusive**.

---

# 3. Non-Goals (For MVP)

The following are **explicitly out of scope**:

* Streaming transcription
* Editing transcript before insertion
* Voice commands
* Multiple languages
* Long-form recording
* Conversation mode
* Cloud integrations
* Complex UI

The tool is strictly **dictation → paste text**.

---

# 4. User Flow

### Step 1 — Start Dictation

User presses:

```
Super + I
```

System behavior:

* Recording begins immediately.
* Microphone input uses the **system default audio input device**.
* A **small popup indicator** appears in the bottom-right corner indicating recording is active.

Example popup:

```
🎤 Listening...
```

---

### Step 2 — Stop Dictation

User presses:

```
Super + I
```

again.

System behavior:

* Recording stops.
* Audio recording is finalized.

---

### Step 3 — Transcription

The recorded audio is sent to:

```
Parakeet speech-to-text model
```

Output:

```
raw transcription text
```

Example:

```
hello this is a test message how are you doing today
```

---

### Step 4 — Text Cleanup

The transcription is passed to a **locally running LLM**.

Example model:

```
Gemma 3B
```

The LLM receives:

**System prompt**

```
You are a text cleanup assistant.
Fix punctuation, grammar, and capitalization.
Do not change the meaning of the text.
Return only the cleaned sentence.
```

**Input**

```
hello this is a test message how are you doing today
```

Output:

```
Hello, this is a test message. How are you doing today?
```

---

### Step 5 — Insert Text

The cleaned text is inserted into the user's system.

Behavior:

#### Case A — Cursor is active in an application

The system should:

```
paste the text at the current cursor position
```

Example:

If the cursor is in a text editor, browser, terminal input, etc., the text appears there.

---

#### Case B — No active cursor

If no active text field exists:

```
copy text to clipboard
```

The user can then manually paste it.

---

# 5. Functional Requirements

### FR1 — Global Hotkey

The system must support a **global hotkey**:

```
Super + I
```

Behavior:

* First press → start recording
* Second press → stop recording

---

### FR2 — Microphone Recording

* Use **system default microphone**
* Capture audio until recording stops
* Store temporarily in memory or temp file

---

### FR3 — Speech Recognition

The system must transcribe audio using:

```
NVIDIA Parakeet
```

Requirements:

* Local inference
* GPU acceleration when available

---

### FR4 — Text Cleanup

Transcribed text must be processed through:

```
local LLM
```

The LLM must:

* fix punctuation
* fix capitalization
* correct minor grammar issues

The LLM must **not significantly rewrite content**.

---

### FR5 — Text Output

After processing, the system must:

1. Detect the **currently focused application**
2. Insert the cleaned text at the **current cursor position**

Fallback behavior:

```
if insertion fails → copy to clipboard
```

---

### FR6 — Visual Feedback

The system should display a **minimal popup indicator** when recording.

Example states:

Recording:

```
🎤 Listening...
```

Processing:

```
Processing speech...
```

Completed:

```
✓ Inserted
```

Popup should auto-dismiss.

---

# 6. Performance Requirements

Target latencies:

Recording stop → text inserted:

```
< 2 seconds
```

Breakdown (target):

| Step                   | Target |
| ---------------------- | ------ |
| Audio finalize         | <100ms |
| Parakeet transcription | <800ms |
| LLM cleanup            | <600ms |
| Insertion              | <100ms |

---

# 7. System Constraints

The system assumes:

* Linux desktop environment
* GPU available for local models
* Parakeet installed locally
* Local LLM runtime available

Examples:

* llama.cpp
* Ollama
* vLLM

---

# 8. Privacy

All processing must occur **locally**.

No audio or text data is sent to external services.

---

# 9. Configuration (Optional for MVP)

Config file example:

```
~/.config/appname/config.yaml
```

Possible fields:

```
hotkey: Super+I
asr_model: parakeet
llm_model: gemma3b
paste_method: simulate_keystrokes
```

---

# 10. MVP Definition

The MVP is complete when:

User can:

1. Press **Super + I**
2. Speak a sentence
3. Press **Super + I** again
4. See the **cleaned sentence inserted into the active application**

With **no manual copy/paste required**.

---

# Honest Product Advice

This is a **perfectly scoped MVP**. Don't add more right now.

The real engineering challenges will be:

1. **Global hotkeys on Wayland**
2. **Reliable text insertion**
3. **Fast model startup (daemon vs CLI)**

Everything else is straightforward.

---

If you'd like, I can also show you the **best architecture for this (about 300 lines of code total)** so you avoid over-engineering it.
