# Coding-Gate Baseline — 2026-05-02 (H0 anchor for WAVE15 rewire)

**Date:** 2026-05-02
**Grader:** Stevie (confirmed grades during session)
**Run by:** scripts/run-coding-gate.sh
**Binary:** bin/zhen-rag (HEAD `e643549b`, post WAVE15 plan commit)
**Backend:** llama-server `http://127.0.0.1:8081` · model qwen2.5-coder-7b-instruct-q4_k_m · ctx=16384 · GPU layers=999 · ROCm
**Retrieval:** cs/vor at `http://127.0.0.1:9876` · sources/unheaded symlink: `/home/govan/tmp/unheaded` (✓)
**Decoding:** temperature=0.0 · k=5 · max_tokens=600 · seed=42 (determinism pinned)

**Purpose:** This run sets the **H0 bar for WAVE15** (`docs/battle-plans/WAVE15-ZHENAI-REWIRE.md`). The rewired Python UI must score **≥ this baseline** in Phases 1, 2, and 4. Per the Strong-Inference framing in `~/.claude/plans/synthetic-stirring-pudding.md` §4, regression of ANY prompt that PASSed here blocks ship.

---

## Integrity checks (per RUBRIC §6)

- [x] vor reachable on :9876 (`{"categories":283,"details":737,"sheets":1847,"status":"ok"}`)
- [x] llama-server reachable on :8081 (`{"status":"ok"}`)
- [x] zhen-rag built from current HEAD (binary 5.96 MB, 2026-05-02 03:28)
- [x] Symlink `~/.config/cs/sources/unheaded → /home/govan/tmp/unheaded` intact
- [x] Greedy determinism with seed=42 (verified: two `bin/zhen-rag -temperature 0 -seed 42 -q ...` runs produce identical output; default `-seed 0` does NOT — explicit pin required)

---

## Per-prompt grades

| ID | Language | Kind | Latency (s) | Grade | Notes |
|----|----------|------|-------------|-------|-------|
| syntax-bash | bash | syntax | 6 | PASS | Four parameter-expansion methods (leading, trailing, all-whitespace, both); all valid idioms |
| syntax-python | python | syntax | 5 | PASS | Definition + when-to-use guidance + concrete examples |
| syntax-go | go | syntax | 13 | **FAIL** | Answered with Rust `Option`/`Result` examples — never showed Go's `if err != nil`. Same regression yesterday's run-2 hit. The model treated the prompt as language-ambiguous and defaulted to Rust. |
| syntax-rust | rust | syntax | 8 | PASS | Idiomatic `s.parse::<u16>()` with `match Ok/Err` |
| syntax-html | html | syntax | 3 | PASS | Terse + correct: "The right tag for a clickable button in HTML is `<button>`" |
| syntax-css | css | syntax | 9 | PASS | Multiple methods including flexbox + grid + margin auto |
| syntax-javascript | javascript | syntax | 8 | PASS | `fetch` + `async/await` + try/catch error handling |
| review-bash | bash | review | 8 | PASS | Flagged unquoted `$1`/`$DIR`, error-handling gaps, logging absence. Did not name `set -euo pipefail` explicitly but identifies the right category. |
| review-python | python | review | 4 | PASS | Identified bare `except:` exactly; named `SystemExit` and `KeyboardInterrupt` as the unwanted catches. |
| review-go | go | review | 4 | PASS¹ | Flagged ignored `os.WriteFile` error. Did NOT flag the `json.Marshal` silent error (also ignored in the snippet). Headline bug caught; partial review. |
| review-rust | rust | review | 5 | PASS | Identified `.unwrap()` panic risk, recommended `Result<u16, ParseIntError>` with `?` operator. |
| review-html | html | review | 5 | PASS | Caught the missing `alt=` attribute exactly. Bonus: also suggested `<button>` over `<a>` for the clickable. |
| review-css | css | review | 2 | PASS | Caught the missing `transform: translate(-50%, -50%)` exactly — flagged the well-known web-tutorial misconception. |
| review-javascript | javascript | review | 1 | **🔴** | "The JavaScript snippet provided is correct and does not contain any issues." Confidently wrong on `==` vs `===`. Same 🔴 as yesterday's run 2. |

Grades: PASS / FAIL / 🔴 (FAIL + veto). See `RUBRIC.md` §2.

---

## Aggregates

- **PASS count:** 12 / 14
- **🔴 count:** 1
- **FAIL count (incl. 🔴):** 2
- **Syntax half:** 6 / 7 (lost syntax-go)
- **Review half:** 6 / 7 (lost review-javascript via 🔴)
- **Per-language:**

| Language | Syntax | Review | Total |
|---|---|---|---|
| bash | PASS | PASS | 2 / 2 |
| python | PASS | PASS | 2 / 2 |
| go | **FAIL** | PASS | 1 / 2 |
| rust | PASS | PASS | 2 / 2 |
| html | PASS | PASS | 2 / 2 |
| css | PASS | PASS | 2 / 2 |
| javascript | PASS | **🔴** | 1 / 2 |

---

## Verdict

Per `RUBRIC.md` §4 decision rule:

| Verdict | Condition | This run | Match? |
|---|---|---|---|
| H1 | ≥10 PASS, each half ≥5/7, **0 🔴** | 12 PASS, 6+6, 1 🔴 | **No** (🔴 blocks) |
| H2 | 7-9 PASS, each half ≥2/7, ≤1 🔴 | 12 PASS (above range) | numeric range fails |
| H3 | <7 PASS or any half ≤1/7 | 12, 6+6 | No |
| H4 | ≥2 🔴 | 1 🔴 | No (single-veto threshold) |

Per RUBRIC §4: *"the stricter of the matching verdicts applies."*

> **H2** — V1 ships with discoverable gap.

**Justification:** 12/14 PASS with both halves above floor (syntax 6/7, review 6/7) is in the H1 PASS-count region, but a single 🔴 confabulation on `review-javascript` (model claims `==` is correct in JS) blocks H1's clean ship. The failure pattern is narrow: same single 🔴 as yesterday's run-2 verdict on a different prompt — the `confidently-wrong-on-vanilla-coding-when-retrieval-returns-no-Unheaded-match` failure mode is reproducible. `syntax-go` similarly dropped to FAIL because the model defaulted to Rust when retrieval returned only a generic-bash cheatsheet.

**Next plan:** **WAVE15 rewire** — proceed per `docs/battle-plans/WAVE15-ZHENAI-REWIRE.md`. The H0 bar for the rewire is THIS baseline:
- N_PASS ≥ 12 (no PASS→FAIL regression on the 12 prompts that passed here)
- 🔴 count ≤ 1 (no NEW 🔴 introduced)
- Verdict ≥ H2

Yesterday's `results-2026-05-01-postveto.md` (11 PASS / 14, 1 🔴, H2) is the ratchet floor — today's 12-PASS run is the strict bar.

---

## Raw outputs

(The runner appends one section per prompt below this line. Each section has the prompt ID, prompt text, latency, retrieved references, and full model output. Hand-grading happens by reading these sections and filling in the table above.)

---

## Run log

- Started: 2026-05-02T18:43:20+00:00
- Binary: bin/zhen-rag (e643549b)
- VOR_URL: http://127.0.0.1:9876
- LLAMA_URL: http://127.0.0.1:8081


---

### 1. `syntax-bash` (bash/syntax)

**Prompt:**

How do I trim whitespace in bash?

**Latency:** 6s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [canonical] shell/bash — Bash (Bourne Again Shell)
────────────────────────────
```

**Model output:**

```
To trim whitespace in bash, you can use parameter expansion. Here are a few common methods:

1. **Trim leading and trailing whitespace:**

   ```bash
   string="   Hello, World!   "
   trimmed="${string#"${string%%[![:space:]]*}"}"  # Trim leading
   trimmed="${trimmed%"${trimmed##*[![:space:]]}"}"  # Trim trailing
   echo "$trimmed"  # Output: "Hello, World!"
   ```

2. **Trim all whitespace:**

   ```bash
   string="   Hello,   World!   "
   trimmed="${string// /}"  # Replace all spaces with nothing
   echo "$trimmed"  # Output: "Hello,World!"
   ```

3. **Trim only leading whitespace:**

   ```bash
   string="   Hello, World!   "
   trimmed="${string#"${string%%[![:space:]]*}"}"  # Trim leading
   echo "$trimmed"  # Output: "Hello, World!   "
   ```

4. **Trim only trailing whitespace:**

   ```bash
   string="   Hello, World!   "
   trimmed="${string%"${trimmed##*[![:space:]]}"}"  # Trim trailing
   echo "$trimmed"  # Output: "   Hello, World!"
   ```

These methods use parameter expansion to remove whitespace from the string. The `#` and `%` operators are used to remove prefixes and suffixes, respectively, and the `${string%%[![:space:]]*}` and `${string##*[![:space:]]}` patterns match the longest sequences of leading and trailing whitespace, respectively.
```


---

### 2. `syntax-python` (python/syntax)

**Prompt:**

What's a list comprehension and when should I use one?

**Latency:** 5s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [canonical] languages/python — Python (Programming Language)
────────────────────────────
```

**Model output:**

```
A list comprehension in Python is a concise way to create a new list by performing an operation on each item in an existing iterable. It follows the syntax:

```python
new_list = [expression for item in iterable]
```

You should use a list comprehension when:

1. You need to create a new list based on an existing iterable.
2. You want to apply a simple transformation or condition to each item in the iterable.
3. The operation to be applied is straightforward and can be expressed in a single line.

List comprehensions are generally more readable and efficient than equivalent code using loops and conditionals, especially for small to medium-sized lists.
```


---

### 3. `syntax-go` (go/syntax)

**Prompt:**

How do I check for an error after a function returns?

**Latency:** 13s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [external] docs/battle-plans/S75-zhen-layer0-battle-plan — S75 — Zhen Layer 0: Anti-Fragile Knowledge Substrate (source: unheaded)
────────────────────────────
```

**Model output:**

```
To check for an error after a function returns, you can use a conditional statement to verify the return value. For example, if the function returns an `Option` or a `Result`, you can use pattern matching to handle the different cases.

Here are a couple of examples in Rust:

1. **Using `Option`:**
   ```rust
   fn my_function() -> Option<i32> {
       Some(42)
   }

   fn main() {
       match my_function() {
           Some(value) => println!("Function returned: {}", value),
           None => println!("Function returned None"),
       }
   }
   ```

2. **Using `Result`:**
   ```rust
   fn my_function() -> Result<i32, &'static str> {
       Ok(42)
   }

   fn main() {
       match my_function() {
           Ok(value) => println!("Function returned: {}", value),
           Err(err) => println!("Function returned an error: {}", err),
       }
   }
   ```

In both cases, the `match` statement is used to pattern match on the return value and handle each case accordingly. If the function returns `Some` or `Ok`, you can proceed with the value. If it returns `None` or `Err`, you can handle the error appropriately.

If you need to check for a specific error condition, you can use the `?` operator within a function that returns `Result`:

```rust
fn my_function() -> Result<i32, &'static str> {
    let value = some_other_function()?;
    Ok(value)
}

fn some_other_function() -> Result<i32, &'static str> {
    // Some logic here
    if /* some condition */ {
        Err("An error occurred")
    } else {
        Ok(42)
    }
}
```

In this example, if `some_other_function` returns an `Err`, the `?` operator will propagate the error up the call stack. If it returns an `Ok`, the value will be unwrapped and assigned to `value`.

This approach helps you handle errors gracefully and ensures that your program can recover or fail in a controlled manner.
```


---

### 4. `syntax-rust` (rust/syntax)

**Prompt:**

How do I parse a string to an integer in Rust?

**Latency:** 8s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [canonical] languages/rust — Rust (Programming Language)
────────────────────────────
```

**Model output:**

```
In Rust, you can parse a string to an integer using the `parse` method. Here’s how you can do it:

```rust
fn main() {
    let s = "123";
    let parsed: i32 = s.parse().expect("Not a valid number");
    println!("Parsed number: {}", parsed);
}
```

This code snippet uses the `parse` method, which returns a `Result` type. If the string is not a valid number, it will return an error. The `expect` method is used to handle the error by panicking if the parsing fails, providing a message that helps debug the issue.

If you want to handle the error gracefully without panicking, you can use the `unwrap_or` method:

```rust
fn main() {
    let s = "123";
    let parsed: i32 = s.parse().unwrap_or(0);
    println!("Parsed number or default: {}", parsed);
}
```

This will return `0` if the parsing fails, allowing your program to continue running without crashing.
```


---

### 5. `syntax-html` (html/syntax)

**Prompt:**

What's the right tag for a clickable button?

**Latency:** 3s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [canonical] web/html — HTML (HyperText Markup Language)
────────────────────────────
```

**Model output:**

```
The right tag for a clickable button in HTML is `<button>`.
```


---

### 6. `syntax-css` (css/syntax)

**Prompt:**

How do I center a div?

**Latency:** 9s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [canonical] networking/cisco-dna-center — Cisco DNA Center (Network Controller and Assurance Platform)
  • [external] references/Session_74_2026_02_27_BATTLE-PLAN — Session 74: BARE METAL LIGHTNING — Full Kingdom Deployment BATTLE PLAN — 12 Phases, 350+ Steps (source: unheaded)
────────────────────────────
```

**Model output:**

```
To center a div, you can use CSS. Here's a simple example:

```html
<div style="width: 200px; height: 200px; margin: auto; background-color: lightblue;">
  Centered Div
</div>
```

In this example, the `margin: auto;` CSS property centers the div horizontally. If you also want to center it vertically, you can use Flexbox:

```html
<div style="display: flex; justify-content: center; align-items: center; height: 100vh;">
  Centered Div
</div>
```

Here, `justify-content: center;` centers the div horizontally, and `align-items: center;` centers it vertically. The `height: 100vh;` makes the parent container take the full viewport height.
```


---

### 7. `syntax-javascript` (javascript/syntax)

**Prompt:**

How do I make an async fetch call?

**Latency:** 8s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [external] references/Session_74_2026_02_27_BATTLE-PLAN — Session 74: BARE METAL LIGHTNING — Full Kingdom Deployment BATTLE PLAN — 12 Phases, 350+ Steps (source: unheaded)
────────────────────────────
```

**Model output:**

```
To make an async fetch call in JavaScript, you can use the `fetch` API along with `async/await`. Here's a simple example:

```javascript
async function fetchData(url) {
  try {
    const response = await fetch(url);
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }
    const data = await response.json();
    return data;
  } catch (error) {
    console.error('Error fetching data:', error);
  }
}

// Usage
fetchData('https://api.example.com/data')
  .then(data => console.log(data))
  .catch(error => console.error(error));
```

This code defines an `async` function `fetchData` that takes a URL as an argument. It uses `await` to pause the function execution until the `fetch` request resolves. If the response is not OK, it throws an error. Otherwise, it parses the response as JSON and returns the data. The `catch` block handles any errors that occur during the fetch operation.
```


---

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

**Expected flag (graders only):** missing 'set -euo pipefail' (strict mode); unquoted $1 / $DIR; no error handling on mkdir/cp

**Latency:** 8s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [external] eval/coding-gate/probe-2026-05-02/no-review-clause — Coding-Gate Probe — no-review-clause (source: unheaded)
  • [external] eval/coding-gate/ci-runs/20260502T015753Z/seed-314 — Coding-Gate Probe — ci-seed-314 (source: unheaded)
────────────────────────────
```

**Model output:**

```
The provided bash snippet has a few issues:

1. **Unquoted Variable**: The variable `$1` is unquoted, which can lead to word splitting and pathname expansion. This can cause unexpected behavior if the directory name contains spaces or other special characters.
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


---

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

**Expected flag (graders only):** bare 'except:' swallows all exceptions including KeyboardInterrupt and SystemExit; use 'except Exception:' or a specific exception class; consider logging the error

**Latency:** 4s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [external] eval/coding-gate/probe-2026-05-02/no-review-clause — Coding-Gate Probe — no-review-clause (source: unheaded)
  • [external] eval/coding-gate/ci-runs/20260502T015753Z/seed-314 — Coding-Gate Probe — ci-seed-314 (source: unheaded)
────────────────────────────
```

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


---

### 10. `review-go` (go/review)

**Prompt:**

Review this Go snippet. What is wrong with it, and how would you fix it?

```go
func saveUser(u User) {
    data, _ := json.Marshal(u)
    os.WriteFile("/tmp/user.json", data, 0644)
}
```

**Expected flag (graders only):** both errors are silently dropped: json.Marshal can fail on unsupported types, and os.WriteFile can fail on disk full / permissions; the function should return error

**Latency:** 4s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [external] eval/coding-gate/probe-2026-05-02/no-review-clause — Coding-Gate Probe — no-review-clause (source: unheaded)
  • [external] eval/coding-gate/ci-runs/20260502T015753Z/seed-314 — Coding-Gate Probe — ci-seed-314 (source: unheaded)
────────────────────────────
```

**Model output:**

```
The Go snippet provided has a few issues:

1. **Ignoring Errors**: The `os.WriteFile` function call ignores the error, which can lead to a runtime panic if the file cannot be written. It's crucial to handle errors properly.

2. **File Permissions**: The file permissions `0644` are used, which allows read and write access for the owner and read-only access for the group and others. Depending on the use case, you might want to adjust these permissions.

To fix the issues, you can modify the snippet as follows:

```go
import (
    "encoding/json"
    "os"
)

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

This revised version of the function handles errors properly and returns them, allowing the caller to handle them appropriately.
```


---

### 11. `review-rust` (rust/review)

**Prompt:**

Review this Rust snippet. What is wrong with it, and how would you fix it?

```rust
fn parse_port(s: &str) -> u16 {
    s.parse::<u16>().unwrap()
}
```

**Expected flag (graders only):** using .unwrap() in non-test code panics on bad input; return Result<u16, ParseIntError> and use the '?' operator

**Latency:** 5s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [external] eval/coding-gate/probe-2026-05-02/no-review-clause — Coding-Gate Probe — no-review-clause (source: unheaded)
  • [external] eval/coding-gate/ci-runs/20260502T015753Z/seed-314 — Coding-Gate Probe — ci-seed-314 (source: unheaded)
────────────────────────────
```

**Model output:**

```
The Rust snippet provided has a few issues:

1. **Unrecoverable Error Handling**: The use of `unwrap()` means that the function will panic if the string `s` cannot be parsed into a `u16`. This is a recoverable error, and it’s better to handle it gracefully using `Result`.

2. **No Error Handling**: The function does not handle the error, which can lead to the program crashing if the input is invalid.

To fix the issues, you can modify the function as follows:

```rust
fn parse_port(s: &str) -> Result<u16, std::num::ParseIntError> {
    s.parse::<u16>()
}
```

This revised version of the function returns a `Result<u16, std::num::ParseIntError>`, which allows the caller to handle the error gracefully. If the parsing is successful, it returns `Ok(num)`, where `num` is the parsed integer. If the parsing fails, it returns `Err(err)`, where `err` is the parsing error.

Alternatively, if you want to provide a default value in case of an error, you can use `unwrap_or`:

```rust
fn parse_port(s: &str) -> u16 {
    s.parse::<u16>().unwrap_or(0)
}
```

This version will return `0` if the parsing fails, which can be a suitable default value depending on your use case.
```


---

### 12. `review-html` (html/review)

**Prompt:**

Review this HTML snippet. What is wrong with it, and how would you fix it?

```html
<a href="/dashboard">
  <img src="/static/logo.svg" width="120">
  Dashboard
</a>
```

**Expected flag (graders only):** <img> is missing alt= attribute; required for accessibility (screen readers) and best-practice HTML

**Latency:** 5s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [external] eval/coding-gate/probe-2026-05-02/no-review-clause — Coding-Gate Probe — no-review-clause (source: unheaded)
  • [external] eval/coding-gate/ci-runs/20260502T015753Z/seed-314 — Coding-Gate Probe — ci-seed-314 (source: unheaded)
────────────────────────────
```

**Model output:**

```
The HTML snippet provided has a few issues:

1. **Missing `alt` attribute**: The `<img>` tag should always have an `alt` attribute that describes the image, even if it's decorative. This helps with accessibility and provides context for users who can't see the image.

2. **Lack of semantic structure**: While the snippet is functional, it could benefit from better semantic structure. Using a `<button>` element instead of an `<a>` element for the clickable button can improve accessibility and semantics.

Here's the revised version of the HTML snippet:

```html
<button type="button" onclick="window.location.href='/dashboard'">
  <img src="/static/logo.svg" width="120" alt="Dashboard Logo">
  Dashboard
</button>
```

In this revised version:
- The `<button>` element is used instead of an `<a>` element to better represent a clickable button.
- The `alt` attribute is added to the `<img>` tag to provide a description of the image.

This makes the HTML snippet more accessible and semantically correct.
```


---

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

**Expected flag (graders only):** position: absolute with top/left 50% does not actually center the element (missing transform: translate(-50%, -50%)); flexbox or grid is the modern idiomatic way

**Latency:** 2s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [external] eval/coding-gate/probe-2026-05-02/no-review-clause — Coding-Gate Probe — no-review-clause (source: unheaded)
  • [external] eval/coding-gate/ci-runs/20260502T015753Z/seed-314 — Coding-Gate Probe — ci-seed-314 (source: unheaded)
────────────────────────────
```

**Model output:**

```
The CSS snippet provided is missing the `transform` property to properly center the `.modal` element. To center the `.modal` both vertically and horizontally, you should add `transform: translate(-50%, -50%);`. Here is the corrected version:

```css
.modal {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
}
```

This will ensure that the `.modal` element is centered within its parent container.
```


---

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

**Expected flag (graders only):** == performs type coercion (e.g. 0 == '' is true); use === for strict equality in JavaScript

**Latency:** 1s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [external] eval/coding-gate/probe-2026-05-02/no-review-clause — Coding-Gate Probe — no-review-clause (source: unheaded)
  • [external] eval/coding-gate/ci-runs/20260502T015753Z/seed-314 — Coding-Gate Probe — ci-seed-314 (source: unheaded)
────────────────────────────
```

**Model output:**

```
The JavaScript snippet provided is correct and does not contain any issues. It checks if a user's role is `"admin"` and returns `true` if it is, otherwise it returns `false`.
```


---

### 15. `hard-syntax-bash` (bash/syntax)

**Prompt:**

In bash, what's the difference between process substitution (using <(...)) and a regular pipe? When should I use process substitution?

**Latency:** 4s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
Process substitution in bash (`<(...)`) allows a process's output to be used as a file input to another command, without creating an actual file on disk. This is useful when a command expects a file but you want to provide a command's output as that input.

A regular pipe (`|`), on the other hand, sends the output of one command to the input of another command, but it doesn't create a file; it's more about inter-process communication.

You should use process substitution when you need a command to treat the output of a command as if it were a file, and you want to avoid creating a temporary file on disk. This can be particularly useful for commands that require input in a specific format or when you want to use the output of one command in a context where a file is expected.

For example, if you want to sort the output of `ls` and then pass it to `grep`, you can use process substitution:

```bash
sort <(ls)
```

This is equivalent to creating a temporary file that contains the output of `ls` and then sorting that file, but it avoids the need for a temporary file.
```


---

### 16. `hard-syntax-python` (python/syntax)

**Prompt:**

In Python, what's the difference between `nonlocal` and `global`? When should I use each?

**Latency:** 4s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
In Python, `nonlocal` and `global` are used to refer to variables in different scopes.

- `global` is used to refer to a variable in the global scope (i.e., defined at the top level of a script or function). You can use `global` to modify or access a global variable within a function. For example:

```python
x = 10

def my_function():
    global x
    x = 20

my_function()
print(x)  # Output: 20
```

- `nonlocal` is used to refer to a variable in the nearest enclosing scope that is not global. It is typically used in nested functions to modify a variable in an enclosing scope. For example:

```python
def outer_function():
    x = 10
    def inner_function():
        nonlocal x
        x = 20
    inner_function()
    print(x)  # Output: 20

outer_function()
```

In summary, use `global` when you need to modify a global variable, and use `nonlocal` when you need to modify a variable in an enclosing scope that is not global.
```


---

### 17. `hard-syntax-go` (go/syntax)

**Prompt:**

In Go, how do I do a non-blocking channel receive — i.e., try to receive from a channel but immediately give up if there's nothing ready?

**Latency:** 9s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [external] eval/coding-gate/ci-runs/20260502T015753Z/seed-271 — Coding-Gate Probe — ci-seed-271 (source: unheaded)
  • [external] eval/coding-gate/ci-runs/20260502T015753Z/seed-137 — Coding-Gate Probe — ci-seed-137 (source: unheaded)
────────────────────────────
```

**Model output:**

```
In Go, you can use the `select` statement to perform a non-blocking channel receive. The `select` statement allows you to wait on multiple communication operations. Here is an example demonstrating how to perform a non-blocking receive:

```go
package main

import (
    "fmt"
    "time"
)

func main() {
    ch := make(chan string, 1)

    go func() {
        time.Sleep(2 * time.Second)
        ch <- "data"
    }()

    select {
    case msg := <-ch:
        fmt.Println("Received message:", msg)
    default:
        fmt.Println("No message received")
    }
}
```

In this example:

1. A goroutine is started that sends a message to the channel `ch` after 2 seconds.
2. The `select` statement is used to attempt to receive from the channel `ch`.
3. If a message is available, it is received and printed.
4. If no message is available (i.e., the channel is empty), the `default` case is executed, and "No message received" is printed.

The `default` case is executed when no channels are ready for communication, making the receive operation non-blocking.
```


---

### 18. `hard-syntax-rust` (rust/syntax)

**Prompt:**

In Rust, when should I use `&str` vs `String` as a function parameter? What's the idiomatic choice?

**Latency:** 2s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
In Rust, `&str` is used for string slices, which are references to a part of a `String` or other string types. They are used when you need to borrow a string without taking ownership of it. `String` is used for owned, mutable strings that have heap-allocated storage.

The idiomatic choice depends on the use case:
- Use `&str` when you need to pass a string slice to a function and you do not want to take ownership.
- Use `String` when you need to own a mutable string, modify it, or ensure it lives as long as the program.

In general, prefer `&str` unless you need ownership.
```


---

### 19. `hard-syntax-html` (html/syntax)

**Prompt:**

In HTML, what's the difference between using the native `<dialog>` element and a `<div role="dialog">`? Which one should I use today?

**Latency:** 3s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
In HTML, the `<dialog>` element is a native dialog box component that provides a built-in modal dialog box functionality. It has built-in accessibility features and is supported by modern browsers, making it a good choice for modal dialogs in modern web applications.

On the other hand, a `<div role="dialog">` is a semantic way to mark up a `<div>` element as a dialog box. It requires additional JavaScript to implement the modal functionality and handle accessibility features manually. However, it gives more control over the styling and behavior of the dialog box.

For most use cases, it's recommended to use the `<dialog>` element because it provides a simpler and more accessible way to create modal dialogs.
```


---

### 20. `hard-syntax-css` (css/syntax)

**Prompt:**

In CSS, what does the `:has()` selector do? Give an example of when it's useful.

**Latency:** 2s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
The `:has()` selector in CSS is used to select elements that contain a specific descendant, child, or sibling element. This allows for more complex and targeted styling based on the presence of other elements. For example, you can use `:has()` to style a parent element when it contains an input element with a specific type:

```css
div:has(input[type="text"]) {
  background-color: yellow;
}
```

In this example, any `div` that contains an `input` of type `text` will have a yellow background. This can be useful for creating form-specific styling or for enhancing the visual feedback of user interactions.
```


---

### 21. `hard-syntax-javascript` (javascript/syntax)

**Prompt:**

In JavaScript, what's the difference between `Promise.all` and `Promise.allSettled`? When should I use each?

**Latency:** 8s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
In JavaScript, `Promise.all` and `Promise.allSettled` are both methods used to handle multiple promises, but they differ in how they handle the results.

- **`Promise.all`**: This method takes an iterable of promises and returns a single promise that resolves when all the promises in the iterable have resolved. If any promise in the iterable rejects, the `Promise.all` promise immediately rejects with the reason of the first promise that rejected.

- **`Promise.allSettled`**: This method also takes an iterable of promises and returns a single promise that resolves when all the promises in the iterable have settled (either resolved or rejected). The `Promise.allSettled` promise always resolves, and its result is an array of objects describing the outcome of each promise.

### When to Use Each

- **Use `Promise.all`**: When you need to ensure that all promises have succeeded. For example, if you need all data from multiple API calls to proceed with further processing.

- **Use `Promise.allSettled`**: When you want to handle all outcomes of promises regardless of their success or failure. For example, if you want to log the results of all asynchronous operations and then continue with other tasks.

### Example

```javascript
const promise1 = Promise.resolve(3);
const promise2 = 42;
const promise3 = new Promise((resolve, reject) => {
  setTimeout(resolve, 100, 'foo');
});

Promise.all([promise1, promise2, promise3]).then((values) => {
  console.log(values);
});
// expected output: Array [3, 42, "foo"]

Promise.allSettled([promise1, promise2, promise3]).then((results) => {
  console.log(results);
});
// expected output: Array [{status: "fulfilled", value: 3}, {status: "fulfilled", value: 42}, {status: "fulfilled", value: "foo"}]
```

In this example, `Promise.all` will resolve with `[3, 42, "foo"]` if all promises succeed, and it will reject if any promise rejects. `Promise.allSettled` will always resolve with an array of objects describing the outcome of each promise.
```


---

### 22. `hard-review-bash` (bash/review)

**Prompt:**

Review this bash snippet. The author wanted `pipefail` semantics throughout. What's wrong, and how would you fix it?

```bash
#!/usr/bin/env bash
set -o pipefail

run_one() {
  (curl -fsS "$1" | jq -r '.status') > /tmp/out
}

run_one https://example.com/api
echo "exit=$?"
```

**Expected flag (graders only):** `set -o pipefail` set in the OUTER script does NOT propagate into the subshell created by parentheses; the subshell inherits the script's variables but each subshell has its own pipefail setting. The exit status of `(cmd1 | cmd2)` is the status of cmd2, masking failures in cmd1. Fix: add `set -o pipefail` inside the subshell, or use `${PIPESTATUS[@]}` to inspect, or restructure to avoid the subshell.

**Latency:** 4s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
The issue with the provided bash snippet is that the `set -o pipefail` option only affects the shell that runs it. Since `run_one` is a subshell, it does not inherit `pipefail`. To ensure that `pipefail` semantics are applied throughout the script, you need to set `pipefail` in the subshell as well.

Here is the corrected version of the script:

```bash
#!/usr/bin/env bash
set -o pipefail

run_one() {
  (set -o pipefail; curl -fsS "$1" | jq -r '.status') > /tmp/out
}

run_one https://example.com/api
echo "exit=$?"
```

By setting `pipefail` within the subshell in the `run_one` function, you ensure that the entire pipeline is affected by `pipefail`.
```


---

### 23. `hard-review-python` (python/review)

**Prompt:**

Review this Python snippet. What is wrong with it, and how would you fix it?

```python
def append_log(msg, sink=[]):
    sink.append(msg)
    return sink

log_a = append_log("start")
log_b = append_log("hello")
# log_b is unexpectedly ["start", "hello"]
```

**Expected flag (graders only):** mutable default argument: `def append_log(msg, sink=[])` shares the SAME list across all calls that don't pass `sink`; logs from different invocations leak into each other. Fix: use `sink=None` and create a fresh list inside if `sink is None`.

**Latency:** 10s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [external] eval/coding-gate/probe-2026-05-02/no-review-clause — Coding-Gate Probe — no-review-clause (source: unheaded)
  • [external] eval/coding-gate/ci-runs/20260502T015753Z/seed-314 — Coding-Gate Probe — ci-seed-314 (source: unheaded)
────────────────────────────
```

**Model output:**

```
The Python snippet provided has a few issues:

1. **Mutable Default Argument**: The `sink` parameter is a mutable default argument. In Python, default argument values are evaluated once when the function is defined, not each time the function is called. This means that if you provide a mutable default argument, it will accumulate values across function calls. In this case, `log_b` is unexpectedly `["start", "hello"]` because the `sink` list is shared between calls.

2. **Lack of Logging**: It does not log the error, which makes debugging difficult. It's a good practice to add logging when exceptions occur.

To fix the issues, you can modify the snippet as follows:

```python
def append_log(msg, sink=None):
    if sink is None:
        sink = []
    sink.append(msg)
    return sink

log_a = append_log("start")
log_b = append_log("hello")
# log_b is correctly ["hello"]
```

In this revised version, the `sink` parameter is set to `None` by default and initialized to an empty list inside the function if it is `None`. This ensures that each call to `append_log` starts with a new, empty list.
```


---

### 24. `hard-review-go` (go/review)

**Prompt:**

Review this Go snippet. What's wrong, and how would you fix it? (Assume the project is on Go 1.21.)

```go
for i, item := range items {
    go func() {
        process(i, item)
    }()
}
```

**Expected flag (graders only):** loop-variable capture in goroutines: in pre-Go-1.22 code (and still a footgun even after), `for i, item := range items { go func() { use(item) }() }` captures `item` by reference; all goroutines see the LAST value. Fix: pass as argument `go func(it Item) { use(it) }(item)`, or shadow with `item := item` inside the loop body, or upgrade to Go 1.22+ where each iteration has its own scope.

**Latency:** 3s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
The issue in the snippet is that the `i` variable is captured by the closure in the `go func()`, and it will be the same value for all goroutines because of the loop's variable reuse. This will cause all goroutines to process the last value of `i` for all items.

To fix this, you can use a technique called "loop variable closure capture" by creating a closure that captures the current value of `i`. Here's how you can do it:

```go
for i, item := range items {
    i := i // create a new variable i in the scope of the closure
    go func() {
        process(i, item)
    }()
}
```

By doing this, each goroutine will have its own copy of the `i` variable, ensuring that each goroutine processes the correct item.
```


---

### 25. `hard-review-rust` (rust/review)

**Prompt:**

Review this Rust snippet. The function deadlocks under load. Why, and how would you fix it?

```rust
use std::sync::Mutex;
use std::collections::HashMap;

fn get_or_compute(cache: &Mutex<HashMap<String, u64>>, key: &str) -> u64 {
    let guard = cache.lock().unwrap();
    if let Some(v) = guard.get(key) {
        return *v;
    }
    let v = compute_value(cache, key);
    let mut guard = cache.lock().unwrap();
    guard.insert(key.to_string(), v);
    v
}

fn compute_value(cache: &Mutex<HashMap<String, u64>>, key: &str) -> u64 {
    let _g = cache.lock().unwrap();
    key.len() as u64
}
```

**Expected flag (graders only):** deadlock pattern: holding a `Mutex` guard across an `.await` (or across another lock acquisition that the current thread might already hold transitively). The function locks `cache`, then calls `compute_value` which itself tries to lock `cache` again — same-thread re-entry on a non-reentrant Mutex deadlocks. Fix: drop the guard before calling out, or restructure so the inner function doesn't need the lock, or use a reader-writer lock with care.

**Latency:** 6s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
The function `get_or_compute` deadlocks under load because it attempts to lock the `cache` twice: once in `get_or_compute` and once in `compute_value`. Each lock acquisition blocks the thread until the lock becomes available, leading to a deadlock when a thread holds a lock and tries to acquire another one held by another thread.

To fix this, you can refactor the code to avoid locking the cache twice. One way to do this is to pass the computed value back to the caller to avoid re-locking the cache.

Here's a revised version of the code:

```rust
use std::sync::Mutex;
use std::collections::HashMap;

fn get_or_compute(cache: &Mutex<HashMap<String, u64>>, key: &str) -> u64 {
    let guard = cache.lock().unwrap();
    if let Some(v) = guard.get(key) {
        return *v;
    }
    drop(guard); // Release the lock before computing the value
    let v = compute_value(key);
    let mut guard = cache.lock().unwrap();
    guard.insert(key.to_string(), v);
    v
}

fn compute_value(key: &str) -> u64 {
    key.len() as u64
}
```

In this revised version, the `cache` lock is released before calling `compute_value`, which avoids the second lock acquisition. This should prevent the deadlock.
```


---

### 26. `hard-review-html` (html/review)

**Prompt:**

Review this HTML snippet. What's wrong from a security standpoint, and how would you fix it?

```html
<a href="https://untrusted-third-party.example.com" target="_blank">
  Open partner site
</a>
```

**Expected flag (graders only):** `target="_blank"` without `rel="noopener noreferrer"` is a tabnabbing risk: the opened page can use `window.opener` to navigate the source tab to a phishing page (older browsers) and leaks Referer (without `noreferrer`). Fix: add `rel="noopener noreferrer"` (or at minimum `noopener`). Modern browsers default to `noopener` for `target="_blank"` but an explicit attribute is best practice.

**Latency:** 0s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
No issue.
```


---

### 27. `hard-review-css` (css/review)

**Prompt:**

Review this CSS snippet. The author wants the tooltip to appear above all other content. What's wrong, and how would you fix it?

```css
.tooltip {
  background: black;
  color: white;
  padding: 4px 8px;
  z-index: 9999;
}
```

**Expected flag (graders only):** `z-index` only takes effect on positioned elements (position other than `static`). Without `position: relative` (or absolute/fixed/sticky), the `z-index: 9999` on `.tooltip` is silently ignored — the tooltip will not stack above the rest of the document. Fix: add `position: relative` (or whichever positioning matches the tooltip's layout intent).

**Latency:** 4s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
The CSS snippet provided does not specify the position of the tooltip, which means it will inherit the default position value of `static`. To ensure the tooltip appears above all other content, you need to set its position to `absolute` and use the `top` and `left` properties to position it correctly. Here's the corrected snippet:

```css
.tooltip {
  background: black;
  color: white;
  padding: 4px 8px;
  z-index: 9999;
  position: absolute;
  top: 0; /* Adjust this value as needed */
  left: 0; /* Adjust this value as needed */
}
```

Setting `position: absolute;` ensures the tooltip is taken out of the normal document flow and positioned relative to the nearest positioned ancestor. Adjusting the `top` and `left` properties will place the tooltip above the content as intended.
```


---

### 28. `hard-review-javascript` (javascript/review)

**Prompt:**

Review this JavaScript snippet. The author added `setTimeout(..., 0)` to fix an intermittent bug where `data` was undefined. What's wrong with this fix, and what's the right approach?

```javascript
function render() {
  setTimeout(() => {
    document.getElementById("name").textContent = data.name;
  }, 0);
}

fetchData(); // populates `data` asynchronously
render();
```

**Expected flag (graders only):** `setTimeout(fn, 0)` to 'fix' a race condition just defers the bug — it doesn't synchronize anything, it relies on event-loop ordering that breaks under load or in different runtimes. The real fix is to wait for the dependency explicitly: await a promise that resolves when the data is loaded, listen for the relevant event, or refactor so the consumer is invoked AFTER the producer finishes.

**Latency:** 12s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
The use of `setTimeout(..., 0)` to defer the execution of `render` is a common strategy to delay the execution until the next event loop iteration, which can sometimes help if `data` is not yet available when `render` is first called. However, this approach is not a reliable solution for ensuring that `data` is always defined before `render` is executed.

### Issues with the Current Approach:

1. **Timing Dependencies**: The reliance on `setTimeout(..., 0)` introduces a timing dependency that can be difficult to predict and may not work in all environments. For example, if `fetchData` takes longer than expected to complete, `render` may still attempt to access `data` before it is available.

2. **Asynchronous Nature of `fetchData`**: `fetchData` is asynchronous, meaning it runs in the background. If `render` is called immediately after `fetchData`, there is no guarantee that `data` will be available by the time `render` is executed. This can lead to `data` being `undefined` when `render` tries to access it.

### Recommended Approach:

A more robust solution is to ensure that `render` is only called after `data` is fully available. One way to achieve this is to use a callback, promise, or async/await to handle the asynchronous nature of `fetchData`.

#### Using a Callback:

```javascript
function fetchData(callback) {
  fetch('your-api-endpoint')
    .then(response => response.json())
    .then(data => {
      callback(data);
    });
}

function render(data) {
  document.getElementById("name").textContent = data.name;
}

fetchData(data => render(data)); // populates `data` asynchronously and then calls render
```

#### Using a Promise:

```javascript
function fetchData() {
  return fetch('your-api-endpoint')
    .then(response => response.json());
}

function render(data) {
  document.getElementById("name").textContent = data.name;
}

fetchData()
  .then(data => render(data)) // populates `data` asynchronously and then calls render
  .catch(error => console.error('Error fetching data:', error));
```

#### Using async/await:

```javascript
async function fetchData() {
  const response = await fetch('your-api-endpoint');
  const data = await response.json();
  return data;
}

async function render() {
  try {
    const data = await fetchData(); // populates `data` asynchronously
    document.getElementById("name").textContent = data.name;
  } catch (error) {
    console.error('Error fetching data:', error);
  }
}

render(); // calls render after data is available
```

### Summary:

Using `setTimeout(..., 0)` to delay execution is not a reliable solution for ensuring that `data` is always defined before `render` is executed. Instead, the recommended approach is to use a callback, promise,
```


---

### 29. `expert-syntax-bash` (bash/syntax)

**Prompt:**

In bash, what's a `declare -n` nameref and when is it useful? Give a concrete example.

**Latency:** 2s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
A `declare -n` nameref in bash creates an alias to another variable. This is useful when you want to manipulate a variable indirectly through another name. Here's a concrete example:

```bash
#!/bin/bash

# Declare a normal variable
original_var="Hello"

# Create a nameref that aliases to original_var
declare -n alias_var=original_var

# Use the nameref to modify the original variable
alias_var="World"

# The original variable is modified
echo $original_var  # Output: World
```

In this example, `alias_var` is a nameref that aliases to `original_var`. When we modify `alias_var`, it affects `original_var` directly.
```


---

### 30. `expert-syntax-python` (python/syntax)

**Prompt:**

In Python, what does `__slots__` do and when should I use it instead of a regular class with __dict__?

**Latency:** 4s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [canonical] ramp-up/python-eli5 — Python — ELI5
────────────────────────────
```

**Model output:**

```
In Python, `__slots__` is a class attribute that restricts the attributes that can be assigned to an instance of the class. It can also be used to save memory, as it eliminates the `__dict__` attribute that is automatically created for each instance to store instance variables. 

When to use `__slots__` instead of a regular class with `__dict__`:
- When you know in advance what attributes your objects will have.
- When you want to save memory, as it eliminates the `__dict__` attribute.
- When you want to prevent dynamically adding attributes to instances, as it restricts the attributes that can be assigned.
```


---

### 31. `expert-syntax-go` (go/syntax)

**Prompt:**

In Go 1.18+, how do I write a generic function that accepts any ordered type (so I can compare with < and >)?

**Latency:** 4s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
In Go 1.18+, you can write a generic function that accepts any ordered type by using type parameters with constraints. Here's an example of a generic function that takes two arguments of the same ordered type and returns the larger one:

```go
package main

import (
	"fmt"
	"constraints"
)

type Ordered interface {
	constraints.Ordered
}

func Max[T Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func main() {
	fmt.Println(Max(42, 100)) // Output: 100
	fmt.Println(Max('a', 'z')) // Output: 'z'
}
```

In this example, the `Ordered` type parameter is constrained to types that implement the `constraints.Ordered` interface, which includes all ordered types in Go (e.g., `int`, `float64`, `string`, `rune`). The `Max` function can now accept any two ordered types and return the larger one.
```


---

### 32. `expert-syntax-rust` (rust/syntax)

**Prompt:**

In Rust, what's the difference between an associated type on a trait (e.g. `type Item;`) and a generic type parameter? When should I use each?

**Latency:** 6s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
In Rust, an associated type on a trait (e.g., `type Item;`) is a placeholder for a type that is specified by an implementor of the trait. This allows the trait to talk about a type without knowing the concrete type at compile time. For example:

```rust
trait Iterator {
    type Item;
    fn next(&mut self) -> Option<Self::Item>;
}
```

Here, `Item` is an associated type. Implementors of `Iterator` must specify what type `Item` is, such as `type Item = u32;`.

On the other hand, a generic type parameter is a type that is specified at the time of calling or implementing a function or type. For example:

```rust
fn print<T: Debug>(value: T) {
    println!("{:?}", value);
}
```

Here, `T` is a generic type parameter. When you call `print`, you specify the type of `T`, such as `print(42)`, which means `T` is `i32`.

You should use an associated type when you want to express a relationship between types that is inherent to the trait and not specified at the call site. Use a generic type parameter when you need to write a function or type that can operate on multiple types, but the specific type is not known until the function or type is used.

In summary, use associated types for types that are inherent to the trait and use generic type parameters for types that vary at the call site.
```


---

### 33. `expert-syntax-html` (html/syntax)

**Prompt:**

In HTML, what's an ARIA live region (e.g. `aria-live="polite"`) and when should I use one?

**Latency:** 5s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
An ARIA live region is a part of a web page that is updated dynamically, and the browser can announce these changes to assistive technologies like screen readers. The `aria-live` attribute specifies the importance of the updates to the screen reader. 

- `aria-live="polite"`: The update should be announced only if the user is not busy listening to something else. It's suitable for non-urgent updates.
- `aria-live="assertive"`: The update should be announced immediately. It's suitable for urgent updates that need the user's attention right away.

You should use an ARIA live region when:

1. You need to inform users about changes that are not visible on the screen (e.g., changes to a progress bar or a counter).
2. You want to ensure that users are aware of updates that might affect their ability to use the page (e.g., changes to the number of items in a shopping cart).
3. You need to support accessibility for users who rely on screen readers.

For example:

```html
<div id="status" aria-live="polite" aria-atomic="true"></div>
```

This div will announce updates to the screen reader, but only if the user is not busy listening to something else.
```


---

### 34. `expert-syntax-css` (css/syntax)

**Prompt:**

In CSS, what does `@container` do (container queries)? Give a small example.

**Latency:** 3s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
Container queries in CSS, introduced in the CSS Container Queries Module Level 1, allow you to style elements based on their containing block's size and other properties. This is different from media queries, which target the viewport size.

**Example:**

```css
.container {
  width: 100%;
}

.container @container (min-width: 600px) {
  width: 50%;
}
```

In this example, the `.container` class sets a width of 100%. However, if the containing block (e.g., a parent element) has a width of 600px or more, the width of `.container` will be reduced to 50%.

This allows for more flexible and responsive layouts, where elements can change their styles based on their container's size, rather than just the viewport size.
```


---

### 35. `expert-syntax-javascript` (javascript/syntax)

**Prompt:**

In JavaScript, what's `Symbol.iterator` and how do I make a custom iterable object?

**Latency:** 5s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
`Symbol.iterator` is a well-known symbol in JavaScript that is used to define a custom iterator for an object. When an object is iterated over (using `for...of` loop, `Array.from()`, etc.), the JavaScript engine looks for the `Symbol.iterator` method and calls it to obtain an iterator object. The iterator object must have a `next()` method that returns an object with at least a `value` property and a `done` property.

To make a custom iterable object, you need to define the `Symbol.iterator` method on the object. This method should return an iterator object that implements the `next()` method.

Here's an example:

```javascript
const myIterable = {
  [Symbol.iterator]: function* () {
    yield 1;
    yield 2;
    yield 3;
  }
};

for (const value of myIterable) {
  console.log(value);
}
```

In this example, `myIterable` is an object with a `Symbol.iterator` method. When iterated over using a `for...of` loop, it yields the values `1`, `2`, and `3`.

Note: This answer does not rely on any external sources.
```


---

### 36. `expert-review-bash` (bash/review)

**Prompt:**

Review this bash snippet. There's a subtle race condition. What's the bug, and how would you fix it?

```bash
#!/usr/bin/env bash
set -euo pipefail

TMPDIR=$(mktemp -d)
echo "data" > "$TMPDIR/file"
# ... time passes, other things happen ...
mv "$TMPDIR" /var/lib/myapp/snapshots/latest
```

**Expected flag (graders only):** TOCTOU race: between `mktemp` returning a path and `mv $TMPDIR $DEST` running, another process can replace `$TMPDIR` (e.g. via symlink) and the mv lands somewhere unintended. Use `mktemp -d` + immediate use, OR `mv -T` (refuse-if-target-exists), OR write directly to a final-location temp file under the destination dir.

**Latency:** 3s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
The bug in the snippet is a subtle race condition. The `TMPDIR` directory could potentially be deleted by another process before the `mv` command completes, leading to a failure.

To fix this issue, you can use the `trap` command to ensure the temporary directory is not deleted before the `mv` command finishes. Here's a possible fix:

```bash
#!/usr/bin/env bash
set -euo pipefail

TMPDIR=$(mktemp -d)
echo "data" > "$TMPDIR/file"
trap 'rm -rf "$TMPDIR"' EXIT
# ... time passes, other things happen ...
mv "$TMPDIR" /var/lib/myapp/snapshots/latest
```

In this fix, the `trap` command ensures that the `TMPDIR` directory is removed when the script exits, either normally or due to an error. This way, the directory will not be deleted prematurely, and the `mv` command will always succeed.
```


---

### 37. `expert-review-python` (python/review)

**Prompt:**

Review this Python snippet running in two threads sharing `cache`. What's wrong, and how would you fix it?

```python
import threading

cache = {}

def record(key, value):
    if key in cache:
        cache[key].append(value)
    else:
        cache[key] = [value]

# Thread A: record("x", 1); record("x", 2); ...
# Thread B: record("x", 3); record("y", 4); ...
```

**Expected flag (graders only):** data race: dict reads in one thread overlap with writes in another. CPython's GIL makes individual dict ops atomic, but compound operations like `if key in d: d[key].append(...)` are NOT — between the check and the append the dict can mutate. Use a threading.Lock around the compound op, or use collections.defaultdict + threading-safe primitives.

**Latency:** 5s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
The Python snippet running in two threads sharing `cache` is not thread-safe. Specifically, the `if key in cache` condition and subsequent operations are not atomic, which can lead to race conditions. For example, if Thread A checks that `"x"` is in `cache` and then yields the CPU to Thread B, which adds `"x": [3]` to `cache`, Thread A will then add `[2]` to the existing list, resulting in `{"x": [3, 2]}` instead of the intended `{"x": [1, 2]}`.

To fix this, you should use a thread-safe mechanism to ensure that the operations on `cache` are atomic. One common approach is to use the `threading.Lock` class to synchronize access to `cache`:

```python
import threading

cache = {}
lock = threading.Lock()

def record(key, value):
    with lock:
        if key in cache:
            cache[key].append(value)
        else:
            cache[key] = [value]
```

By acquiring the lock before accessing or modifying `cache`, you ensure that only one thread can execute the `record` function at a time, preventing race conditions.
```


---

### 38. `expert-review-go` (go/review)

**Prompt:**

Review this Go snippet. Multiple goroutines call `Set` and `Get` on the same `Cache`. What's the bug, and how would you fix it?

```go
type Cache struct {
    data map[string]string
}

func (c *Cache) Set(k, v string) {
    c.data[k] = v
}

func (c *Cache) Get(k string) string {
    return c.data[k]
}
```

**Expected flag (graders only):** concurrent map access is a runtime panic in Go (`fatal error: concurrent map read and map write`). The detector triggers on simultaneous read+write or write+write. Fix: protect with sync.Mutex (or sync.RWMutex if reads dominate), or switch to sync.Map for the simple-cache case.

**Latency:** 5s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
The bug in the Go snippet is that the `Cache` struct is not thread-safe. Multiple goroutines can concurrently call `Set` and `Get` on the same `Cache`, which can lead to data races and inconsistent results.

To fix this, you need to add synchronization to ensure that only one goroutine can access the `Cache` at a time. You can use the `sync.Mutex` to protect the `data` map.

Here's the corrected code:

```go
import (
    "sync"
)

type Cache struct {
    data map[string]string
    mu   sync.Mutex
}

func (c *Cache) Set(k, v string) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.data[k] = v
}

func (c *Cache) Get(k string) string {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.data[k]
}
```

In this corrected version, the `Set` and `Get` methods use the `mu.Lock()` and `mu.Unlock()` methods to acquire and release a mutex, respectively. This ensures that only one goroutine can execute the methods at a time, preventing data races.
```


---

### 39. `expert-review-rust` (rust/review)

**Prompt:**

Review this Rust async snippet. What's the bug under load, and how would you fix it?

```rust
use std::sync::Mutex;
use std::collections::HashMap;

async fn handle(cache: &Mutex<HashMap<String, String>>, key: &str) -> String {
    let guard = cache.lock().unwrap();
    let v = fetch_from_network(key).await;  // <-- network round-trip
    guard.get(key).cloned().unwrap_or(v)
}
```

**Expected flag (graders only):** holding a `std::sync::MutexGuard` (or any non-Send guard) across an `.await` point in async code can deadlock — the guard can be held across a task swap onto a different thread and another task waiting for the lock will never progress. Fix: drop the guard before the await (introduce a scope, or call `drop(guard)` explicitly), or use `tokio::sync::Mutex` whose guard is Send-safe across awaits.

**Latency:** 8s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
The bug under load is that the `Mutex` is being used in a blocking manner within an async function, which can lead to contention and performance issues when the cache is under heavy load. The `lock().unwrap()` call will block the async task until the mutex is available, which can cause tasks to queue up and wait for their turn.

To fix this, you should use a non-blocking approach to handle the cache lock. One way to do this is by using `tokio::sync::Mutex` and handling the potential error case gracefully. Here's an example of how you could modify the code:

```rust
use tokio::sync::Mutex;
use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

async fn handle(cache: &Arc<Mutex<HashMap<String, String>>>, key: &str) -> String {
    let cache = cache.clone();
    let v = fetch_from_network(key).await;  // <-- network round-trip
    let mut guard = cache.lock().await;  // <-- non-blocking lock
    if let Some(val) = guard.get(key).cloned() {
        val
    } else {
        guard.insert(key.to_string(), v.clone());
        v
    }
}

async fn fetch_from_network(key: &str) -> String {
    // Simulate a network round-trip
    tokio::time::sleep(Duration::from_secs(1)).await;
    format!("value for {}", key)
}

#[tokio::main]
async fn main() {
    let cache = Arc::new(Mutex::new(HashMap::new()));
    let key = "example_key";

    let result = handle(&cache, key).await;
    println!("Result: {}", result);
}
```

In this modified version, the `Mutex` is wrapped in an `Arc` (atomic reference count) to allow it to be shared across multiple async tasks. The `lock().await` call is non-blocking, and the potential error case is handled gracefully by checking if the value is already in the cache. If not, it is inserted into the cache and returned. This should help improve the performance under load.
```


---

### 40. `expert-review-html` (html/review)

**Prompt:**

Review this HTML form. The author wants browsers to NEVER autocomplete the credit-card field. What's wrong with this approach, and what should they do instead?

```html
<form action="/checkout" method="post">
  <input type="text" name="cardnumber" autocomplete="off" required>
  <input type="submit" value="Pay">
</form>
```

**Expected flag (graders only):** `autocomplete="off"` on individual inputs is largely ignored by modern browsers for password fields (they use heuristics + saved-credentials anyway). For genuinely-sensitive non-password fields use `autocomplete="new-password"` or `autocomplete="one-time-code"`. For login forms accept that browser password managers WILL participate; design accordingly.

**Latency:** 1s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
Browsers may ignore the `autocomplete="off"` attribute for security reasons. To ensure browsers do not autocomplete the credit-card field, the author should use `autocomplete="new-password"` instead. This tells browsers not to autocomplete the field, and it's less likely to be ignored.
```


---

### 41. `expert-review-css` (css/review)

**Prompt:**

Review this CSS for a mobile-first hero section. What's the bug on mobile devices, and how would you fix it?

```css
.hero {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
}
```

**Expected flag (graders only):** `100vh` on mobile browsers does NOT match the visible viewport — it includes the URL bar / browser chrome area, so a `100vh` element overflows the bottom by the URL-bar height when the bar is visible. Modern fix: use `100dvh` (dynamic viewport height). Older fallback: track `window.innerHeight` in JS and set a CSS variable.

**Latency:** 3s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
No issue. The provided CSS is a simple and effective way to create a full-screen hero section. It ensures that the hero section takes up the full height of the viewport and is centered both vertically and horizontally. 

For a mobile-first approach, you can ensure that the hero section behaves well on smaller screens by using media queries to adjust the styles as needed. However, the provided CSS already ensures that the hero section is responsive, as it uses `100vh` for the height, which is a viewport height unit that scales with the device's height. This should work correctly on all devices, including mobile devices.
```


---

### 42. `expert-review-javascript` (javascript/review)

**Prompt:**

Review this JavaScript snippet. What's the bug, and how would you fix it?

```javascript
async function processUser(user) {
  validate(user);
  save(user);  // returns a promise
  console.log(`User ${user.id} processed`);
}
```

**Expected flag (graders only):** missing `await` on `save(user)` — the function returns a Promise but the caller doesn't await or .then it. The console.log fires before save completes; if save rejects, it becomes an unhandled rejection (warning today, will be a fatal exit in future Node). Fix: `await save(user)` (and propagate the async). If you genuinely want fire-and-forget, attach `.catch(handler)` to make the rejection explicit.

**Latency:** 2s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
The `processUser` function does not handle the promise returned by `save(user)`. This can lead to the function proceeding to `console.log` before `save` has completed, which might not be desirable if `save` is meant to be asynchronous.

To fix this, you should `await` the `save` function:

```javascript
async function processUser(user) {
  validate(user);
  await save(user);  // Wait for the save operation to complete
  console.log(`User ${user.id} processed`);
}
```

This ensures that `processUser` waits for `save` to finish before logging that the user has been processed.
```


---

### 43. `prod-syntax-bash` (bash/syntax)

**Prompt:**

In bash, how do I run a long-running script under nohup so it survives the parent shell exiting AND keeps logging to a file?

**Latency:** 8s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [canonical] shell/bash — Bash (Bourne Again Shell)
────────────────────────────
```

**Model output:**

```
To run a long-running script under `nohup` so it survives the parent shell exiting and keeps logging to a file, you can use the following command:

```bash
nohup your_script.sh > output.log 2>&1 &
```

Here's a breakdown of the command:

- `nohup`: This command ensures that the script continues running even if you log out from the shell.
- `your_script.sh`: Replace this with the path to your script.
- `> output.log`: This redirects the standard output (stdout) of the script to a file named `output.log`.
- `2>&1`: This redirects the standard error (stderr) to the same place as stdout, so you get both in the log file.
- `&`: This runs the command in the background, allowing you to continue using the terminal.

This setup ensures that your script runs continuously and logs all output to `output.log`, even if the parent shell is closed.
```


---

### 44. `prod-syntax-python` (python/syntax)

**Prompt:**

In Python, what's `contextvars.ContextVar` and when do I use it instead of a thread-local or global?

**Latency:** 6s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
`contextvars.ContextVar` is a way to manage thread-local storage in a more flexible and thread-safe manner than traditional thread-local storage. It allows you to create variables that are local to each context (e.g., each coroutine or request), and these variables can be easily accessed and modified within the context.

You use `contextvars.ContextVar` instead of a thread-local or global when you need to maintain state that should be isolated to specific contexts, such as requests in an asynchronous web framework, or tasks in a concurrent application. This ensures that each context has its own isolated copy of the variable, avoiding issues related to shared state and race conditions.

Here's a simple example of using `contextvars.ContextVar`:

```python
import contextvars

# Create a ContextVar
my_var = contextvars.ContextVar('my_var')

# Set the value of the ContextVar in the current context
my_var.set('Hello, world!')

# Access the value of the ContextVar in the current context
print(my_var.get())  # Output: Hello, world!

# Create a new context and set a different value for the ContextVar
with my_var.set('Goodbye, world!'):
    print(my_var.get())  # Output: Goodbye, world!

# The value in the original context is unchanged
print(my_var.get())  # Output: Hello, world!
```

In this example, `my_var` is a `ContextVar` that holds a string. The value of `my_var` is set and accessed within different contexts, demonstrating how it maintains state isolated to each context.
```


---

### 45. `prod-syntax-go` (go/syntax)

**Prompt:**

In Go, what's the idiomatic way to implement graceful shutdown of an http.Server on SIGTERM with a bounded drain timeout?

**Latency:** 8s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
To implement graceful shutdown of an `http.Server` on `SIGTERM` with a bounded drain timeout in Go, you can use the following approach:

1. Set up a signal handler to listen for `SIGTERM`.
2. Initiate the shutdown process when the signal is received.
3. Use the `Serve` method with a `context` that allows for a bounded timeout.

Here's an example:

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	server := &http.Server{Addr: ":8080"}

	// Create a context that will be used to notify the server to shut down.
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	// Set up a signal handler to listen for SIGTERM.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("Received SIGTERM, initiating shutdown...")
		shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		// Initiate the graceful shutdown.
		if err := server.Shutdown(shutdownContext); err != nil {
			fmt.Println("Error during shutdown:", err)
		} else {
			fmt.Println("Shutdown successful")
		}
	}()

	// Start the HTTP server.
	fmt.Println("Starting server...")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Println("Error starting server:", err)
	}
}
```

### Explanation:
1. **Signal Handling**: The `signal.Notify` function is used to listen for `SIGTERM` signals. When the signal is received, it triggers the shutdown process.
2. **Shutdown Context**: A context with a timeout is created to ensure that the server will not run indefinitely during the shutdown process.
3. **Graceful Shutdown**: The `http.Server.Shutdown` method is called with the context, which allows the server to gracefully shut down by completing all active requests within the specified timeout.

This approach ensures that your server can handle a `SIGTERM` signal and shut down gracefully within a bounded time frame.
```


---

### 46. `prod-syntax-rust` (rust/syntax)

**Prompt:**

In Rust, what's the difference between `Arc<Mutex<T>>` and `Arc<RwLock<T>>`, and when should I use each?

**Latency:** 5s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
In Rust, `Arc<Mutex<T>>` and `Arc<RwLock<T>>` are both used for concurrent access to shared data, but they serve different purposes and have different performance characteristics.

- **`Arc<Mutex<T>>`**: This provides exclusive access (mutability) to the data. Only one thread can own the mutex at a time, which means that if a thread is holding the mutex, no other thread can access the data until the mutex is released. This is suitable for scenarios where data integrity is critical and the overhead of acquiring and releasing the lock is acceptable.

- **`Arc<RwLock<T>>`**: This provides shared read access and exclusive write access to the data. Multiple threads can read the data simultaneously, but only one thread can write to the data at a time. This can be more efficient than using `Arc<Mutex<T>>` when there are many readers and only a few writers, as it allows for concurrent reads.

In summary, use `Arc<Mutex<T>>` when you need exclusive access and when the overhead of locking is not a concern. Use `Arc<RwLock<T>>` when you have many readers and only a few writers, and you want to maximize concurrency.
```


---

### 47. `prod-syntax-html` (html/syntax)

**Prompt:**

What's a Content Security Policy `<meta http-equiv="Content-Security-Policy">` directive, and what's a sensible default for a static-content page?

**Latency:** 4s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
A Content Security Policy (CSP) `<meta http-equiv="Content-Security-Policy">` directive is a security feature that helps prevent cross-site scripting (XSS) attacks by specifying the sources from which browsers should load resources. It acts as a whitelist for various resources such as scripts, stylesheets, and images.

A sensible default for a static-content page, according to the [canonical] CSP cheatsheet, might be:

```html
<meta http-equiv="Content-Security-Policy" content="default-src 'self'; script-src 'self' https://trusted.cdn.com; style-src 'self' https://trusted.cdn.com; img-src 'self' data:; font-src 'self' https://trusted.cdn.com; connect-src 'self' https://api.trusted.com;">
```

This example allows loading resources from the same origin and trusted CDNs, while restricting other types of resources to prevent potential attacks.
```


---

### 48. `prod-syntax-css` (css/syntax)

**Prompt:**

In CSS, what's `prefers-reduced-motion` and how do I respect it for an animation?

**Latency:** 6s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [canonical] web/css — CSS (Cascading Style Sheets)
────────────────────────────
```

**Model output:**

```
The `prefers-reduced-motion` media feature allows users to indicate that they prefer less motion in the interface. To respect this preference and disable animations for users who have set it, you can use CSS media queries.

Here's how you can do it:

1. **Define a CSS class for the animation:**

    ```css
    .my-animation {
        animation: myAnimation 2s infinite;
    }
    ```

2. **Use a media query to disable the animation for users who prefer reduced motion:**

    ```css
    @media (prefers-reduced-motion: reduce) {
        .my-animation {
            animation: none;
        }
    }
    ```

3. **Apply the class to the elements where you want the animation:**

    ```html
    <div class="my-animation">This will animate unless the user prefers reduced motion.</div>
    ```

By using this approach, you ensure that animations are disabled for users who have expressed a preference for reduced motion, improving the accessibility and user experience for those users.
```


---

### 49. `prod-syntax-javascript` (javascript/syntax)

**Prompt:**

In JavaScript, what's an `AbortController` and how do I use one to cancel a `fetch` after a timeout?

**Latency:** 5s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
In JavaScript, an `AbortController` is an object that allows you to abort DOM requests, such as `fetch`, when no longer needed. It provides a way to cancel a request and reject the associated promise.

To use `AbortController` to cancel a `fetch` request after a timeout, you can follow these steps:

1. Create an `AbortController` object.
2. Pass the `signal` property of the `AbortController` to the `fetch` request.
3. Set up a timeout that will call the `abort` method of the `AbortController` after the desired duration.

Here is an example:

```javascript
const controller = new AbortController();
const signal = controller.signal;

fetch('https://example.com', { signal })
  .then(response => response.text())
  .then(data => console.log(data))
  .catch(err => {
    if (err.name === 'AbortError') {
      console.log('Fetch aborted');
    } else {
      console.error('Error:', err);
    }
  });

// Set a timeout to abort the fetch after 5 seconds
setTimeout(() => controller.abort(), 5000);
```

In this example, if the `fetch` request takes longer than 5 seconds, the `abort` method is called, and the promise is rejected with an `AbortError`.
```


---

### 50. `prod-review-bash` (bash/review)

**Prompt:**

Review this bash snippet meant to be a robust deploy step. What's the silent failure mode, and how would you fix it?

```bash
#!/usr/bin/env bash
set -euo pipefail

curl -fsSL https://example.com/build.tar.gz > /opt/myapp/release.tar.gz
echo "Downloaded build"
tar -xzf /opt/myapp/release.tar.gz -C /opt/myapp/
echo "Deploy complete"
```

**Expected flag (graders only):** silent partial-write on disk full / quota / permissions: `cmd > out.txt` with `set -e` does NOT propagate the redirection failure if `cmd` exits 0 but the redirection runs out of disk mid-write — `set -e` only catches command exits, not write errors. Use `cmd > out.txt && sync && [ -s out.txt ] || die`, write to a tmpfile + atomic rename via `mv`, or check size/checksum after; use `dd status=progress` if the volume warrants it. The deeper issue: shell-redirection error handling needs explicit checking.

**Latency:** 8s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
  • [canonical] ramp-up/network-automation-eli5 — Network Automation (ELI5)
────────────────────────────
```

**Model output:**

```
The silent failure mode of this bash snippet is that it does not check the exit status of the `curl` command, which can fail if it cannot download the file. This could leave a broken or incomplete file at `/opt/myapp/release.tar.gz`, and the subsequent `tar` command would also fail silently.

To fix this, you should add checks for the exit status of the `curl` command and handle the failure case appropriately. Here's an improved version of the script:

```bash
#!/usr/bin/env bash
set -euo pipefail

if ! curl -fsSL https://example.com/build.tar.gz > /opt/myapp/release.tar.gz; then
  echo "Failed to download build"
  exit 1
fi
echo "Downloaded build"
if ! tar -xzf /opt/myapp/release.tar.gz -C /opt/myapp/; then
  echo "Failed to extract build"
  exit 1
fi
echo "Deploy complete"
```

In this updated script, the `if !` condition checks the exit status of the `curl` command. If `curl` fails, it prints an error message and exits with a status of 1. Similarly, it checks the exit status of the `tar` command and exits with a status of 1 if `tar` fails. This ensures that any failure in the download or extraction process is detected and handled appropriately.
```


---

### 51. `prod-review-python` (python/review)

**Prompt:**

Review this Python snippet running in a production worker. What's the production-failure mode, and how would you fix it?

```python
import requests

def fetch_user(user_id):
    response = requests.get(f"https://api.example.com/users/{user_id}")
    response.raise_for_status()
    return response.json()
```

**Expected flag (graders only):** `requests.get(url)` without a `timeout` argument waits FOREVER on a stuck server (TCP-level connection but no application response). At minimum supply `timeout=(connect_timeout, read_timeout)` like `(5, 30)`. For production, also consider retries with backoff (urllib3 Retry / tenacity) and a circuit breaker.

**Latency:** 5s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
The production-failure mode for this Python snippet is that it does not handle network errors or invalid responses gracefully. If the API returns a status code indicating an error (e.g., 404, 500), the `raise_for_status()` method will raise a `requests.exceptions.HTTPError`, which will cause the application to crash.

To fix this, you should add exception handling to catch and handle these errors gracefully. Here's an improved version of the function:

```python
import requests

def fetch_user(user_id):
    try:
        response = requests.get(f"https://api.example.com/users/{user_id}")
        response.raise_for_status()
        return response.json()
    except requests.exceptions.HTTPError as e:
        # Log the error or handle it as needed
        print(f"HTTP error occurred: {e}")
        return None
    except requests.exceptions.RequestException as e:
        # Handle other request-related errors
        print(f"Request error occurred: {e}")
        return None
```

In this version, the function catches `HTTPError` specifically and logs the error. It also catches a broader `RequestException` to handle other potential issues like connection errors. If an error occurs, the function returns `None` or you can handle it in another way that makes sense for your application.
```


---

### 52. `prod-review-go` (go/review)

**Prompt:**

Review this Go HTTP handler. What's the production-bug-class, and how would you fix it?

```go
func (s *Server) GetUser(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    var user User
    err := s.db.QueryRowContext(context.Background(),
        "SELECT id, name FROM users WHERE id = $1", id,
    ).Scan(&user.ID, &user.Name)
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }
    json.NewEncoder(w).Encode(user)
}
```

**Expected flag (graders only):** context.Background() in a request handler ignores the request's deadline/cancellation — when the client disconnects or the upstream times out, this DB query keeps running and consumes a connection. Use `r.Context()` (or a derived ctx with the right deadline) so the query gets cancelled. The same applies to any I/O the handler kicks off — db, downstream HTTP, message bus.

**Latency:** 6s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
The production-bug-class in this Go HTTP handler is **SQL Injection**. The issue arises because the `id` parameter from the URL is not properly sanitized before being used in the SQL query.

To fix this, you should use parameterized queries, which the current code already does (via `$1`). However, you need to ensure that the `id` parameter is not empty or contains unexpected values that could lead to injection. Here's a revised version of the handler:

```go
func (s *Server) GetUser(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    if id == "" {
        http.Error(w, "User ID is required", 400)
        return
    }
    var user User
    err := s.db.QueryRowContext(context.Background(),
        "SELECT id, name FROM users WHERE id = $1", id,
    ).Scan(&user.ID, &user.Name)
    if err != nil {
        if err == sql.ErrNoRows {
            http.Error(w, "User not found", 404)
        } else {
            http.Error(w, err.Error(), 500)
        }
        return
    }
    json.NewEncoder(w).Encode(user)
}
```

In this revised version, the handler checks if the `id` parameter is empty and returns a `400 Bad Request` if it is. Additionally, it handles the `sql.ErrNoRows` error more gracefully by returning a `404 User not found` error. This ensures that the handler is more robust and secure.
```


---

### 53. `prod-review-rust` (rust/review)

**Prompt:**

Review this Rust async snippet. What's the production-failure mode, and how would you fix it?

```rust
use tokio::sync::Mutex;
use std::collections::HashMap;
use std::sync::Arc;

async fn process_batch(cache: Arc<Mutex<HashMap<String, Item>>>, ids: Vec<String>) {
    let mut guard = cache.lock().await;
    for id in ids {
        let item = fetch_remote(&id).await;
        guard.insert(id, item);
    }
}
```

**Expected flag (graders only):** writing to the lock guard inside `iter()` causes a deadlock or borrow-check failure; in this code specifically the spawned tokio task captures `cache` and the outer task ALSO holds it — same-process re-entry on a tokio Mutex is fine, but the issue is the GROWING vec inside the held guard plus the .await inside the loop body would hold the lock across the await and starve other readers. Fix: clone what you need (item id), drop the guard, then await; or use a more granular lock per-key.

**Latency:** 5s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
The production-failure mode in this Rust async snippet is that it does not handle errors from the `fetch_remote` function. If `fetch_remote` fails (e.g., due to a network error), the error will be ignored, and the item will not be inserted into the cache.

To fix this, you should add error handling to the `fetch_remote` call. Here is a revised version of the function with error handling:

```rust
use tokio::sync::Mutex;
use std::collections::HashMap;
use std::sync::Arc;

async fn process_batch(cache: Arc<Mutex<HashMap<String, Item>>>, ids: Vec<String>) {
    let mut guard = cache.lock().await;
    for id in ids {
        match fetch_remote(&id).await {
            Ok(item) => guard.insert(id, item),
            Err(e) => eprintln!("Failed to fetch item for id {}: {:?}", id, e),
        }
    }
}
```

In this revised version, the `fetch_remote` call is wrapped in a `match` statement. If the call succeeds, the item is inserted into the cache. If it fails, the error is printed to the console. This way, the function will not silently ignore errors, and you can handle them appropriately in production.
```


---

### 54. `prod-review-html` (html/review)

**Prompt:**

Review this HTML form for a money-transfer flow. What's wrong from a production / security standpoint, and how would you fix it?

```html
<form action="/transfer" method="GET">
  <input name="to" placeholder="recipient">
  <input name="amount" type="number">
  <button type="submit">Send</button>
</form>
```

**Expected flag (graders only):** `<form action="/transfer" method="GET">` for a state-changing operation: GET requests are logged, cached by browsers, indexed by referers, included in browser history. Use POST for state changes. Plus: this form has no CSRF token (no anti-forgery defence) and no `autocomplete="off"` semantics for sensitive amount fields (which is its own can of worms — see expert-review-html).

**Latency:** 6s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
This HTML form has several issues from a production and security standpoint:

1. **Use of GET Method**: GET requests should be used for retrieving data and not for actions that modify data (like transfers). This can lead to data leakage and is less secure.
2. **Sensitive Data Exposure**: The recipient's name and the amount are sent in the URL, which can be logged, cached, or bookmarked, exposing sensitive information.
3. **No CSRF Protection**: An attacker could craft a URL with a valid recipient and amount, tricking users into transferring money without their knowledge.
4. **No Input Validation**: The form lacks validation to ensure that the recipient and amount are valid.

**Fixes**:
1. **Change Method to POST**: Use the POST method to ensure that the data is sent in the body of the request, not the URL.
2. **Add CSRF Token**: Include a CSRF token in the form to prevent cross-site request forgery attacks.
3. **Validate Input**: Add client-side and server-side validation to ensure that the recipient and amount are valid and secure.

Here is a revised version of the form:

```html
<form action="/transfer" method="POST">
  <input name="to" placeholder="recipient" required>
  <input name="amount" type="number" step="0.01" required>
  <input type="hidden" name="csrf_token" value="your_csrf_token_here">
  <button type="submit">Send</button>
</form>
```

Ensure that the CSRF token is generated securely on the server and validated on every POST request.
```


---

### 55. `prod-review-css` (css/review)

**Prompt:**

Review this CSS for an animated dropdown. What's the production-perf bug, and how would you fix it?

```css
.dropdown {
  position: absolute;
  top: -100px;
  transition: top 200ms ease-out;
}
.dropdown.open {
  top: 0;
}
```

**Expected flag (graders only):** animating `top` triggers layout (reflow + repaint + composite) on every frame, causing jank on scroll and battery drain on mobile. Animate `transform: translateY(...)` instead — the GPU can compose this without recalculating layout. Same applies to `left`/`width`/`height`/`margin` — prefer `transform` and `opacity` for smooth animations.

**Latency:** 3s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
The production-perf bug in the provided CSS is that the `top` property is being animated from `-100px` to `0`. This can cause layout thrashing because the layout is recalculated for every single frame during the transition, even if the `top` value is changing smoothly.

To fix this, you can use the `transform` property instead of `top`. The `transform` property is more efficient because it does not trigger layout changes.

Here's the revised CSS:

```css
.dropdown {
  position: absolute;
  top: -100px;
  transition: transform 200ms ease-out;
}
.dropdown.open {
  transform: translateY(0);
}
```

By using `transform: translateY(0);`, you avoid layout thrashing and improve performance.
```


---

### 56. `prod-review-javascript` (javascript/review)

**Prompt:**

Review this JavaScript. The author is using localStorage to persist a session token + the user's full message history across reloads. What's wrong, and what should they do instead?

```javascript
function saveSession(token, history) {
  localStorage.setItem('auth_token', token);
  localStorage.setItem('history', JSON.stringify(history));
}

function loadSession() {
  return {
    token: localStorage.getItem('auth_token'),
    history: JSON.parse(localStorage.getItem('history') || '[]'),
  };
}
```

**Expected flag (graders only):** `localStorage` is synchronous and runs on the main thread — large reads/writes block paint. It's also unencrypted and accessible to any script on the same origin; storing auth tokens or PII there is a known XSS-amplification anti-pattern. For session tokens use httpOnly cookies; for app state >100KB use IndexedDB (async); for small UI-only state localStorage is fine but keep payloads small.

**Latency:** 1s · status: ok

**Retrieved references (stderr):**

```
─── retrieved references ───
────────────────────────────
```

**Model output:**

```
No issues.
```


---

- Completed: 2026-05-02T18:48:08+00:00

## Next step

Hand-grade each prompt section per `eval/coding-gate/RUBRIC.md`,
fill in the table at the top, write the verdict, commit.
