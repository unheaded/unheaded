# Coding-Gate Results — &lt;date&gt;

**Date:** 2026-05-02
**Grader:** Stevie
**Run by:** scripts/run-coding-gate.sh
**Binary:** bin/zhen-rag (HEAD f9311d29)
**Backend:** llama-server http://127.0.0.1:20103 · model qwen2.5-coder-7b-instruct-q4_k_m · ctx=16384 · GPU layers=999
**Retrieval:** cs/vor at http://127.0.0.1:20103 · sources/unheaded symlink: &lt;target&gt;
**Decoding:** temperature=0.0 · k=5 · max_tokens=600

---

## Integrity checks (per RUBRIC §6)

- [ ] vor reachable on :9876
- [ ] llama-server reachable on :8081, ctx=16384
- [ ] zhen-rag built from current HEAD
- [ ] Smoke prompt grounded (WAVE14 H6 returns the session doc reference)
- [ ] Greedy determinism (same prompt twice → identical output)

---

## Per-prompt grades

| ID | Language | Kind | Latency (s) | Grade | Notes |
|----|----------|------|-------------|-------|-------|
| syntax-bash | bash | syntax | _ | _ | _ |
| syntax-python | python | syntax | _ | _ | _ |
| syntax-go | go | syntax | _ | _ | _ |
| syntax-rust | rust | syntax | _ | _ | _ |
| syntax-html | html | syntax | _ | _ | _ |
| syntax-css | css | syntax | _ | _ | _ |
| syntax-javascript | javascript | syntax | _ | _ | _ |
| review-bash | bash | review | _ | _ | _ |
| review-python | python | review | _ | _ | _ |
| review-go | go | review | _ | _ | _ |
| review-rust | rust | review | _ | _ | _ |
| review-html | html | review | _ | _ | _ |
| review-css | css | review | _ | _ | _ |
| review-javascript | javascript | review | _ | _ | _ |

Grades: PASS / FAIL / 🔴 (FAIL + veto). See `RUBRIC.md` §2.

---

## Aggregates

- **PASS count:** _ / 14
- **🔴 count:** _
- **Syntax half:** _ / 7
- **Review half:** _ / 7
- **Per-language**:

| Language | Syntax | Review | Total |
|---|---|---|---|
| bash | _ | _ | _ / 2 |
| python | _ | _ | _ / 2 |
| go | _ | _ | _ / 2 |
| rust | _ | _ | _ / 2 |
| html | _ | _ | _ / 2 |
| css | _ | _ | _ / 2 |
| javascript | _ | _ | _ / 2 |

---

## Verdict

Per `RUBRIC.md` §4 decision rule:

> _ (one of: H1 / H2 / H3 / H4)

**Justification:** _ (1-2 sentences citing the aggregate counts above)

**Next plan:** _ (one of: Phase D-A agent runtime / Phase D-B narrow LoRA / Phase D-C base swap / Phase D-veto system-prompt hardening)

---

## Raw outputs

(The runner appends one section per prompt below this line. Each section has the prompt ID, prompt text, latency, retrieved references, and full model output. Hand-grading happens by reading these sections and filling in the table above.)

## Run log

- Started: 2026-05-02T20:44:47+00:00
- Target: WAVE15 Phase 2 H0 — webui → zhen-agentd → vor + llama-server
- Chat path: PROXIED through cmd/zhen-agentd /api/v1/agent/ask (T6 closed)
- Underlying inference: llama-server qwen-coder-7b at :8081
- Underlying retrieval: vor at :9876 (daemon's own retrieve)
- Decoding: temperature=0.0, seed=42, max_tokens=600

---

## Raw outputs

### 1. `syntax-bash` (bash/syntax)

**Prompt:**

How do I trim whitespace in bash?

**Latency:** 3s · model: qwen2.5-coder-7b-instruct@agentd

**Model output:**

```
Use parameter expansion: `${var#${var%%[![:space:]]*}}` for leading, `${var%${var##*[![:space:]]}}` for trailing. Example:
```bash
str="   hi   "
trimmed=${str#${str%%[![:space:]]*}}
trimmed=${trimmed%${trimmed##*[![:space:]]}}
```
```

### 2. `syntax-python` (python/syntax)

**Prompt:**

What's a list comprehension and when should I use one?

**Latency:** 3s · model: qwen2.5-coder-7b-instruct@agentd

**Model output:**

```
A list comprehension is a concise way to create lists in Python. It should be used when you need to create a new list based on existing iterables and some conditional logic. For example, to create a list of squares of even numbers from 1 to 10, you can use the following list comprehension:
```python
squares = [x**2 for x in range(1, 11) if x % 2 == 0]
```
```

### 3. `syntax-go` (go/syntax)

**Prompt:**

How do I check for an error after a function returns?

**Latency:** 5s · model: qwen2.5-coder-7b-instruct@agentd

**Model output:**

```
In Rust, you can check for an error after a function returns by using pattern matching or the `?` operator if the function returns a `Result`. For example:
```rust
fn read_file(path: &str) -> Result<String, std::io::Error> {
    std::fs::read_to_string(path)
}

fn main() {
    match read_file("example.txt") {
        Ok(content) => println!("File content: {}", content),
        Err(e) => eprintln!("Error reading file: {}", e),
    }
}
```
```

### 4. `syntax-rust` (rust/syntax)

**Prompt:**

How do I parse a string to an integer in Rust?

**Latency:** 2s · model: qwen2.5-coder-7b-instruct@agentd

**Model output:**

```
Use the `parse` method. Example:
```rust
let s = "42";
let num: i32 = s.parse().unwrap();
```
```

### 5. `syntax-html` (html/syntax)

**Prompt:**

What's the right tag for a clickable button?

**Latency:** 1s · model: qwen2.5-coder-7b-instruct@agentd

**Model output:**

```
The right tag for a clickable button is `<button>`. Example:
```html
<button>Click me</button>
```
```

### 6. `syntax-css` (css/syntax)

**Prompt:**

How do I center a div?

**Latency:** 4s · model: qwen2.5-coder-7b-instruct@agentd

**Model output:**

```
To center a div horizontally and vertically, you can use CSS flexbox or CSS Grid. Here's an example using flexbox:

```html
<div class='container'>
  <div class='centered-div'>Centered Content</div>
</div>
```

```css
.container {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100vh; /* Full height of the viewport */
}

.centered-div {
  /* Additional styling for the centered div */
}
```
```

### 7. `syntax-javascript` (javascript/syntax)

**Prompt:**

How do I make an async fetch call?

**Latency:** 3s · model: qwen2.5-coder-7b-instruct@agentd

**Model output:**

```
Use the `fetch` API with `async/await` in JavaScript. Example:
```javascript
async function fetchData(url) {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error('Network response was not ok ' + response.statusText);
  }
  return await response.json();
}
```
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

**Latency:** 3s · model: qwen2.5-coder-7b-instruct@agentd

**Model output:**

```
```bash
#!/usr/bin/env bash

NAME=$1
DIR=/tmp/$NAME

if mkdir -p $DIR; then
    cp -r ./files/* $DIR
    echo "deployed to $DIR"
else
    echo "Failed to create directory $DIR"
    exit 1
fi
```
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

**Latency:** 4s · model: qwen2.5-coder-7b-instruct@agentd

**Model output:**

```
Here's a revised version of the function that catches specific exceptions and logs them:

```python
import json
import logging

logging.basicConfig(level=logging.ERROR)

def load_config(path):
    try:
        with open(path) as f:
            return json.load(f)
    except json.JSONDecodeError as e:
        logging.error(f'JSONDecodeError: {e}')
        return {}
    except FileNotFoundError as e:
        logging.error(f'FileNotFoundError: {e}')
        return {}
    except Exception as e:
        logging.error(f'Unexpected error: {e}')
        return {}
```
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

**Latency:** 3s · model: qwen2.5-coder-7b-instruct@agentd

**Model output:**

```
Fix the function as follows:

```go
func saveUser(u User) error {
    data, err := json.Marshal(u)
    if err != nil {
        return err
    }
    err = os.WriteFile("/tmp/user.json", data, 0644)
    if err != nil {
        return err
    }
    return nil
}
```
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

**Latency:** 6s · model: qwen2.5-coder-7b-instruct@agentd

**Model output:**

```
To fix the issue, you can use a `match` statement or the `?` operator to handle the `Result` returned by `parse()`. Here is an example using `match`:

```rust
fn parse_port(s: &str) -> Result<u16, std::num::ParseIntError> {
    match s.parse::<u16>() {
        Ok(num) => Ok(num),
        Err(e) => Err(e),
    }
}
```

Or using the `?` operator:

```rust
fn parse_port(s: &str) -> Result<u16, std::num::ParseIntError> {
    s.parse::<u16>()?
}
```

These approaches provide better error handling than using `unwrap()`.
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

**Latency:** 2s · model: qwen2.5-coder-7b-instruct@agentd

**Model output:**

```
<a href="/dashboard"><img src="/static/logo.svg" width="120" alt="Dashboard Logo">Dashboard</a>
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

**Latency:** 1s · model: qwen2.5-coder-7b-instruct@agentd

**Model output:**

```
The CSS snippet is correct for centering a modal both horizontally and vertically.
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

**Latency:** 1s · model: qwen2.5-coder-7b-instruct@agentd

**Model output:**

```
Use `===` for strict equality in JavaScript. `==` does type coercion (e.g. `0 == ''` is true), which is rarely what you want.
```

