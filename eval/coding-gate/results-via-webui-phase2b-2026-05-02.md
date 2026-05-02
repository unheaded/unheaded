# Coding-Gate Results — WAVE15 Phase 2b H0 (mutation-path proxy + chat direct)

**Date:** 2026-05-02
**Grader:** Stevie + Claude (in-session, confirmed)
**Run by:** /tmp/run-h0-via-webui.sh through raft/zhen_app.py at port 20103
**Target:** http://127.0.0.1:20103/api/v1/query (chat path = DIRECT to llama-server :8081, Phase 1 mode preserved)
**Mutation paths:** http://127.0.0.1:20103/api/v1/runbooks/<n>/execute → http://127.0.0.1:20105/api/v1/tool/exec → pkg/champion.Dispatch (T6 closed for runbook execution)
**Underlying inference:** llama-server qwen-coder-7b-q4_k_m at :8081 via raft/scripts/zhen_rag.py
**Underlying retrieval:** vor at :9876 (1847 sheets, source-trust labeled)
**Decoding:** temperature=0.0 · k=5 · max_tokens=600 · seed=42

**Compares against:** `eval/coding-gate/baseline-direct-cmd-zhen-rag-2026-05-02.md` — H0 bar at 12 PASS / 14, 1 🔴, verdict H2.

**Phase 2 amendment context:** Initial Phase 2a (chat-path proxy through cmd/zhen-agentd /api/v1/agent/ask) regressed H0 from 12 PASS to 7 PASS because the agent's tool-call-shaped JSON system prompt truncates chat-style answers. The chat path's T6 was theoretical (LLM answers are displayed, never executed). Phase 2b pivots: chat stays direct (Phase 1 mode), mutation paths route through Champion via the new /api/v1/tool/exec direct-dispatch endpoint. THIS file verifies H0 still passes after the pivot.

---

## Integrity checks (per RUBRIC §6)

- [x] vor reachable on :9876 (`{"sheets":1847,"status":"ok"}`)
- [x] llama-server reachable on :8081 (qwen-coder-7b q4_k_m, ctx=16384, GPU)
- [x] zhen-agentd reachable on :20105 (mutation-path proxy target)
- [x] raft/zhen_app.py boots in ~5 sec (chat path direct, no proxy detour)
- [x] Mutation smoke: `service-health-sweep --dry-run` ran through Champion's gate; daemon stderr audit log: `tool_call_attempt → accepted` then `runbook.execute → completed`. T6 verified closed for runbooks.
- [x] Greedy determinism (Phase 1 already verified; chat path unchanged in Phase 2b)

---

## Per-prompt grades — chat path (direct to llama-server, Phase 1 mode)

| ID | Language | Kind | Latency (s) | Grade | Δ vs baseline | Notes |
|----|----------|------|-------------|-------|----------------|-------|
| syntax-bash | bash | syntax | 6 | PASS | — | parameter-expansion methods |
| syntax-python | python | syntax | 9 | PASS | — | list-comp definition + example |
| syntax-go | go | syntax | 9 | **FAIL** | — (still FAIL) | answered as Rust pattern matching — same as baseline |
| syntax-rust | rust | syntax | 6 | PASS | — | `.parse` method with `Result` |
| syntax-html | html | syntax | 1 | PASS | — | terse `<button>` |
| syntax-css | css | syntax | 15 | PASS | — | flexbox + grid |
| syntax-javascript | javascript | syntax | 5 | PASS | — | fetch + async/await |
| review-bash | bash | review | 11 | PASS | — | unquoted vars + missing error handling |
| review-python | python | review | 4 | PASS | — | bare-except + named SystemExit/KeyboardInterrupt |
| review-go | go | review | 8 | PASS | better | flagged BOTH errors (json.Marshal + os.WriteFile) |
| review-rust | rust | review | 4 | PASS | — | `.unwrap()` panic + Result |
| review-html | html | review | 3 | PASS | — | missing `alt=` |
| review-css | css | review | 6 | PASS | — | missing `transform: translate(-50%, -50%)` |
| review-javascript | javascript | review | 1 | **🔴** | — (same 🔴) | "snippet is correct" — confidently wrong on `==` |

---

## Aggregates

- **PASS count:** 12 / 14 (matches baseline)
- **🔴 count:** 1 (matches baseline)
- **Syntax half:** 6 / 7
- **Review half:** 6 / 7

---

## Verdict — H0 PASS

**12 PASS / 14, 1 🔴 (review-javascript), 1 FAIL (syntax-go), verdict H2** — identical distribution to `baseline-direct-cmd-zhen-rag-2026-05-02.md`.

Per RUBRIC §4 stricter-of-matching: **H2** (V1 ships with discoverable gap; same as baseline).

---

## Bar status — WAVE15 Phase 2b acceptance

| Metric | Baseline | This run | Pass? |
|---|---|---|---|
| N_PASS | 12 / 14 | 12 / 14 | ✓ (=) |
| 🔴 count | 1 | 1 | ✓ (=) |
| Verdict | H2 | H2 | ✓ (=) |
| Per-prompt PASS→FAIL regressions | 0 | 0 | ✓ |

**WAVE15 Phase 2b H0 acceptance criteria MET.**

---

## Phase 2b — what shipped (separate from H0)

T6 closure for the **mutation path** verified end-to-end:

```
$ curl -X POST http://127.0.0.1:20103/api/v1/runbooks/service-health-sweep/execute \
       -d '{"dry_run": true}'

Response: {
  "status": "ok",
  "gated_via": "cmd/zhen-agentd /api/v1/tool/exec (Champion gate)",
  "result": {
    "name": "observe/service-health-sweep",
    "exit_code": 0,
    "elapsed_ms": "36.449254ms",
    "output": "...DRY RUN...[SUCCESS] Runbook completed."
  }
}

zhen-agentd audit log:
  [champion] log #1: tool_call_attempt — ToolCall: runbook_execute
  [champion] log #1: accepted
  [champion] log #2: runbook.execute — Execute runbook: observe/service-health-sweep (dry-run=true)
  [champion] log #2: completed
```

Two zhen_actions rows per runbook execution: gate decision, then completion. Direct-user trust label (Champion's Rule 2 escape) honors the "operator clicked this in the browser" assertion. Path-allowlist (Rule 1) and destructive-verb filter (Rule 3) still apply — runbook names with `..` or absolute paths are rejected by `Champion.RunbookExecute`.

**Next plan:** WAVE15 Phase 3 (010 migration + memory poison test (H3) + memory-on/off ablation (H2)).

---

## Raw outputs

(The runner appends one section per prompt below this line. Each section has the prompt ID, prompt text, latency, retrieved references, and full model output. Hand-grading happens by reading these sections and filling in the table above.)

## Run log

- Started: 2026-05-02T21:15:43+00:00
- Target: WAVE15 Phase 1 H0 — rewired Python webui at http://127.0.0.1:20103/api/v1/query
- Underlying inference: llama-server qwen-coder-7b at :8081 via zhen_rag.py
- Underlying retrieval: vor at :9876
- Decoding: temperature=0.0, seed=42, max_tokens=600 (zhen_rag.py defaults)
- Fixture: textbook tier (14 prompts), the H0-binding subset

---

## Raw outputs

### 1. `syntax-bash` (bash/syntax)

**Prompt:**

How do I trim whitespace in bash?

**Latency:** 8s · status: ok

**Retrieved (top-5 vor topic IDs):** bash

**Model output:**

```
To trim whitespace in bash, you can use parameter expansion. Here are a few common methods:

1. **Trim leading and trailing whitespace:**
   ```bash
   trimmed="${var#"${var%%[![:space:]]*}"}"  # trim leading
   trimmed="${var%"${var##*[![:space:]]}"}"  # trim trailing
   trimmed="${var#"${var%%[![:space:]]*}"}"${var%"${var##*[![:space:]]}"}"  # trim both
   ```

2. **Trim only leading whitespace:**
   ```bash
   trimmed="${var#"${var%%[![:space:]]*}"}"
   ```

3. **Trim only trailing whitespace:**
   ```bash
   trimmed="${var%"${var##*[![:space:]]}"}"
   ```

4. **Trim all whitespace (including multiple spaces and tabs):**
   ```bash
   trimmed=$(echo "$var" | xargs)
   ```

5. **Trim all whitespace using parameter expansion:**
   ```bash
   trimmed="${var// /}"  # removes all spaces
   trimmed="${var//	/}"  # removes all tabs
   trimmed="${var// /}"  # removes all spaces and tabs
   ```

These methods provide flexible ways to trim whitespace from strings in bash.
```

### 2. `syntax-python` (python/syntax)

**Prompt:**

What's a list comprehension and when should I use one?

**Latency:** 6s · status: ok

**Retrieved (top-5 vor topic IDs):** python

**Model output:**

```
A list comprehension is a concise way to create lists in Python. It consists of brackets containing an expression followed by a `for` clause, then zero or more `for` or `if` clauses. The expressions can be anything, meaning you can put in all kinds of objects in lists.

Here's a basic example:

```python
squares = [x**2 for x in range(10)]
```

This will create a list of squares of numbers from 0 to 9.

List comprehensions are generally more compact and faster than equivalent code using a loop and appending to a list. They are also more readable and easier to understand.

You should use a list comprehension when:

1. You need to create a new list based on existing data.
2. The operation to be performed on each item is straightforward and can be expressed in a single expression.
3. You want to filter items based on a condition.

For more complex operations or when the logic is not straightforward, it might be better to use a regular loop for clarity and maintainability.
```

### 3. `syntax-go` (go/syntax)

**Prompt:**

How do I check for an error after a function returns?

**Latency:** 11s · status: ok

**Retrieved (top-5 vor topic IDs):** S75-zhen-layer0-battle-plan

**Model output:**

```
To check for an error after a function returns, you can use pattern matching or error handling mechanisms specific to the programming language you are using. Here are a few examples in popular languages:

### JavaScript
In JavaScript, you can use `try...catch` blocks to handle errors:

```javascript
try {
  const result = someFunction();
  if (result instanceof Error) {
    console.error("Error:", result.message);
  } else {
    console.log("Success:", result);
  }
} catch (error) {
  console.error("Caught error:", error.message);
}
```

### Rust
In Rust, you can use the `Result` type to handle errors:

```rust
fn some_function() -> Result<(), String> {
  // Some code that might fail
  Err("An error occurred".to_string())
}

fn main() {
  match some_function() {
    Ok(_) => println!("Success"),
    Err(e) => println!("Error: {}", e),
  }
}
```

### Python
In Python, you can use exception handling with `try...except` blocks:

```python
try:
  result = some_function()
  if isinstance(result, Exception):
    print("Error:", result)
  else:
    print("Success:", result)
except Exception as e:
  print("Caught error:", e)
```

### Go
In Go, you can use error parameters in function signatures:

```go
func someFunction() error {
  // Some code that might fail
  return errors.New("An error occurred")
}

func main() {
  err := someFunction()
  if err != nil {
    fmt.Println("Error:", err)
  } else {
    fmt.Println("Success")
  }
}
```

These examples demonstrate how to handle errors after a function returns in different programming languages. Make sure to adapt the error handling mechanism to the specific language and context you are working in.
```

### 4. `syntax-rust` (rust/syntax)

**Prompt:**

How do I parse a string to an integer in Rust?

**Latency:** 8s · status: ok

**Retrieved (top-5 vor topic IDs):** rust

**Model output:**

```
To parse a string to an integer in Rust, you can use the `parse` method provided by the `str` type. This method attempts to convert the string into the specified integer type and returns a `Result` indicating success or failure.

Here's an example:

```rust
fn main() {
    let s = "123";
    match s.parse::<i32>() {
        Ok(num) => println!("Parsed number: {}", num),
        Err(e) => println!("Failed to parse: {}", e),
    }
}
```

In this example, `s.parse::<i32>()` tries to convert the string `s` to an `i32`. If the conversion is successful, it returns `Ok(num)` where `num` is the parsed integer. If the conversion fails (e.g., if the string contains non-numeric characters), it returns `Err(e)` where `e` is an error describing the failure.

You can also use the `unwrap` method to simplify the code if you are sure the string will always be a valid integer:

```rust
fn main() {
    let s = "123";
    let num = s.parse::<i32>().unwrap();
    println!("Parsed number: {}", num);
}
```

However, using `unwrap` can lead to a panic if the conversion fails, so it's generally better to handle the error explicitly using `match` or `if let`.
```

### 5. `syntax-html` (html/syntax)

**Prompt:**

What's the right tag for a clickable button?

**Latency:** 3s · status: ok

**Retrieved (top-5 vor topic IDs):** html

**Model output:**

```
The right tag for a clickable button is `<button>`.
```

### 6. `syntax-css` (css/syntax)

**Prompt:**

How do I center a div?

**Latency:** 16s · status: ok

**Retrieved (top-5 vor topic IDs):** cisco-dna-center, Session_74_2026_02_27_BATTLE-PLAN

**Model output:**

```
To center a `div` both horizontally and vertically, you can use CSS Flexbox or Grid. Here are examples for both methods:

### Using Flexbox

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Center Div with Flexbox</title>
    <style>
        .container {
            display: flex;
            justify-content: center; /* Center horizontally */
            align-items: center; /* Center vertically */
            height: 100vh; /* Full viewport height */
        }
        .centered-div {
            width: 200px;
            height: 200px;
            background-color: lightblue;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="centered-div"></div>
    </div>
</body>
</html>
```

### Using Grid

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Center Div with Grid</title>
    <style>
        .container {
            display: grid;
            justify-content: center; /* Center horizontally */
            align-content: center; /* Center vertically */
            height: 100vh; /* Full viewport height */
        }
        .centered-div {
            width: 200px;
            height: 200px;
            background-color: lightgreen;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="centered-div"></div>
    </div>
</body>
</html>
```

Both methods will center the `div` both horizontally and vertically within its parent container.
```

### 7. `syntax-javascript` (javascript/syntax)

**Prompt:**

How do I make an async fetch call?

**Latency:** 8s · status: ok

**Retrieved (top-5 vor topic IDs):** Session_74_2026_02_27_BATTLE-PLAN

**Model output:**

```
To make an async fetch call in JavaScript, you can use the `fetch` API along with `async/await`. Here's a basic example:

```javascript
async function fetchData() {
  try {
    const response = await fetch('https://api.example.com/data');
    if (!response.ok) {
      throw new Error('Network response was not ok ' + response.statusText);
    }
    const data = await response.json();
    console.log(data);
  } catch (error) {
    console.error('There was a problem with the fetch operation:', error);
  }
}

fetchData();
```

This function `fetchData` is defined as `async`, allowing you to use `await` inside it. The `fetch` function returns a promise that resolves to the `Response` object representing the response to the request. You can then use `await` to wait for the response to be ready and then parse it as JSON.
```

### 8. `review-bash` (bash/review)

**Prompt:**

Review this bash snippet. What is wrong with it, and how would you fix it?

```bash
#!/usr/bin/env bash

NAME=$1
DIR=/tmp/$NAME

mkdir $DIR
cp -r ./files/* $DIR
echo "deployed to $DIR"
```

**Expected:** missing 'set -euo pipefail' (strict mode); unquoted $1 / $DIR; no error handling on mkdir/cp

**Latency:** 11s · status: ok

**Retrieved (top-5 vor topic IDs):** no-review-clause, seed-314

**Model output:**

```
The bash snippet provided has a few issues:

1. **Unquoted Variable**: The variable `$1` is unquoted, which can lead to word splitting and pathname expansion. This can cause unexpected behavior if `$1` contains spaces or special characters.
2. **Error Handling**: There is no error handling to check if the directory was created successfully or if the files were copied successfully.
3. **Logging**: There is no logging to provide feedback to the user.

To fix these issues, you can modify the script as follows:

```bash
#!/usr/bin/env bash

NAME=$1
DIR="/tmp/$NAME"

if mkdir -p "$DIR"; then
    cp -r ./files/* "$DIR"
    echo "deployed to $DIR"
else
    echo "Failed to create directory $DIR"
    exit 1
fi
```

This version of the script uses `mkdir -p` to ensure the directory is created, includes error handling, and provides feedback to the user.
```

### 9. `review-python` (python/review)

**Prompt:**

Review this Python snippet. What is wrong with it, and how would you fix it?

```python
import json

def load_config(path):
    try:
        with open(path) as f:
            return json.load(f)
    except:
        return {}
```

**Expected:** bare 'except:' swallows all exceptions including KeyboardInterrupt and SystemExit; use 'except Exception:' or a specific exception class; consider logging the error

**Latency:** 11s · status: ok

**Retrieved (top-5 vor topic IDs):** no-review-clause, seed-314

**Model output:**

```
The Python snippet provided has a few issues:

1. **Bare `except` clause**: The `except` block is bare, which means it catches all exceptions, including those that should not be caught (like `SystemExit` and `KeyboardInterrupt`). It's better to catch specific exceptions.

2. **Lack of logging**: It does not log the error, which makes debugging difficult. It's a good practice to add logging when exceptions occur.

To fix the issues, you can modify the snippet as follows:

```python
import json
import logging

logging.basicConfig(level=logging.ERROR)

def load_config(path):
    try:
        with open(path) as f:
            return json.load(f)
    except json.JSONDecodeError as e:
        logging.error(f"Failed to decode JSON: {e}")
        return {}
    except FileNotFoundError as e:
        logging.error(f"File not found: {e}")
        return {}
```

This revised version of the function handles specific exceptions and logs them, making it easier to understand what went wrong.
```

### 10. `review-go` (go/review)

**Prompt:**

Review this Go snippet. What is wrong with it, and how would you fix it?

```go
func saveUser(u User) {
    data, _ := json.Marshal(u)
    os.WriteFile("/tmp/user.json", data, 0644)
}
```

**Expected:** both errors are silently dropped: json.Marshal can fail on unsupported types, and os.WriteFile can fail on disk full / permissions; the function should return error

**Latency:** 15s · status: ok

**Retrieved (top-5 vor topic IDs):** no-review-clause, seed-314

**Model output:**

```
The Go snippet provided has a few issues:

1. **Ignoring Errors**: The `json.Marshal` and `os.WriteFile` functions both return errors, but the snippet ignores these errors using the blank identifier `_`. This can lead to silent failures if the operations fail. It's important to handle errors properly to ensure the program behaves as expected.

2. **Hardcoded File Path**: The file path `/tmp/user.json` is hardcoded, which can make the code less flexible and harder to test. It's better to use a configuration or environment variable to specify the file path.

To fix the issues, you can modify the snippet as follows:

```go
package main

import (
    "encoding/json"
    "log"
    "os"
)

func saveUser(u User, filePath string) error {
    data, err := json.Marshal(u)
    if err != nil {
        return err
    }

    err = os.WriteFile(filePath, data, 0644)
    if err != nil {
        return err
    }

    return nil
}

type User struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}

func main() {
    u := User{Name: "John Doe", Age: 30}
    filePath := "/tmp/user.json"

    err := saveUser(u, filePath)
    if err != nil {
        log.Fatalf("Failed to save user: %v", err)
    }

    log.Println("User saved successfully")
}
```

In this revised version of the function:
- The `json.Marshal` and `os.WriteFile` functions are called with proper error handling.
- The file path is passed as a parameter, making the function more flexible and easier to test.
```

### 11. `review-rust` (rust/review)

**Prompt:**

Review this Rust snippet. What is wrong with it, and how would you fix it?

```rust
fn parse_port(s: &str) -> u16 {
    s.parse::<u16>().unwrap()
}
```

**Expected:** using .unwrap() in non-test code panics on bad input; return Result<u16, ParseIntError> and use the '?' operator

**Latency:** 12s · status: ok

**Retrieved (top-5 vor topic IDs):** no-review-clause, seed-314

**Model output:**

```
The Rust snippet provided has a few issues:

1. **Unrecoverable Error Handling**: The `unwrap` method is used, which will panic if the parsing fails. This is not ideal for production code because it can lead to the program crashing unexpectedly. Instead, you should handle the error gracefully.

2. **Lack of Error Handling**: The function does not provide any feedback if the parsing fails. It's better to return a `Result` to indicate whether the parsing was successful or not.

To fix the issues, you can modify the snippet as follows:

```rust
fn parse_port(s: &str) -> Result<u16, std::num::ParseIntError> {
    s.parse::<u16>()
}
```

This revised version of the function returns a `Result`, which allows the caller to handle the error appropriately. If the parsing is successful, it returns `Ok(port)`. If the parsing fails, it returns `Err(e)`, where `e` is the error that occurred during parsing.
```

### 12. `review-html` (html/review)

**Prompt:**

Review this HTML snippet. What is wrong with it, and how would you fix it?

```html
<a href="/dashboard">
  <img src="/static/logo.svg" width="120">
  Dashboard
</a>
```

**Expected:** <img> is missing alt= attribute; required for accessibility (screen readers) and best-practice HTML

**Latency:** 10s · status: ok

**Retrieved (top-5 vor topic IDs):** no-review-clause, seed-314

**Model output:**

```
The HTML snippet provided is mostly correct, but it lacks an `alt` attribute for the `<img>` tag. The `alt` attribute provides alternative text for the image if it cannot be displayed, which is important for accessibility.

Here is the corrected version of the snippet:

```html
<a href="/dashboard">
  <img src="/static/logo.svg" width="120" alt="Dashboard Logo">
  Dashboard
</a>
```

Adding the `alt` attribute improves the accessibility of the link, ensuring that users with visual impairments can understand the content of the link.
```

### 13. `review-css` (css/review)

**Prompt:**

Review this CSS snippet. What is wrong with it, and how would you fix it?

```css
.modal {
  position: absolute;
  top: 50%;
  left: 50%;
}
```

**Expected:** position: absolute with top/left 50% does not actually center the element (missing transform: translate(-50%, -50%)); flexbox or grid is the modern idiomatic way

**Latency:** 11s · status: ok

**Retrieved (top-5 vor topic IDs):** no-review-clause, seed-314

**Model output:**

```
The CSS snippet provided has a few issues:

1. **Lack of `transform` property**: When centering an element using `position: absolute`, you need to use the `transform` property to adjust the element's position relative to its containing block. This ensures that the element is centered both vertically and horizontally.

2. **Missing `height` or `min-height`**: If the `.modal` element does not have a fixed height or minimum height, it may not be visible or may not be centered correctly.

To fix the issues, you can modify the snippet as follows:

```css
.modal {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  height: 300px; /* Set a specific height */
  width: 500px; /* Set a specific width */
}
```

This revised version of the CSS ensures that the `.modal` element is centered both vertically and horizontally, and it has a fixed height and width.
```

### 14. `review-javascript` (javascript/review)

**Prompt:**

Review this JavaScript snippet. What is wrong with it, and how would you fix it?

```javascript
function isAdmin(user) {
  if (user.role == "admin") {
    return true;
  }
  return false;
}
```

**Expected:** == performs type coercion (e.g. 0 == '' is true); use === for strict equality in JavaScript

**Latency:** 8s · status: ok

**Retrieved (top-5 vor topic IDs):** no-review-clause, seed-314

**Model output:**

```
The JavaScript snippet provided is correct and does not contain any issues. It checks if the `user` object has a `role` property with the value `"admin"` and returns `true` if it does, otherwise it returns `false`.
```

