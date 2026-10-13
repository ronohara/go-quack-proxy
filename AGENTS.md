<!-- BEGIN:nextjs-agent-rules -->
# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` before writing any code. Heed deprecation notices.
<!-- END:nextjs-agent-rules -->

**Rule**: Any new or revised frontend code that shows user-visible text MUST create a `translations.ts` file in the module directory (using the `TranslationKey` pattern) and use `useTranslate()` for every rendered string — page titles, headings, button labels, error messages, placeholders, status text, confirmations, empty states, and badges. Do not ship a new page or component with hardcoded English strings. This applies equally to backend Python code: every user-facing string in endpoints, error responses, and generated output must go through `t()` with a key defined in the module's `translations.py`. This is not optional — it is a requirement for every new or modified file that produces user-visible output.

**Rule — Persistent Errors**: Every error shown to the user in the frontend must be persistent — rendered inline in the component that triggered it, not delivered solely as a transient toast notification. A toast alone is not enough; the error text must remain visible on screen until the user takes action that clears it (e.g., fixing the input and resubmitting). Toasts may accompany the persistent error as supplementary feedback, but they are never the primary or only error display. When an API call or operation fails, store the error message in component state and render it as a visible element (banner, inline alert, or form-level message) near the relevant input or action area.

**Rule — Bug Fix Workflow**: All bug fixes and code changes follow this sequence. Every step through step 7 happens on the **x360 development machine** — never on a514. Deployment to production (step 8) is a separate, optional step that requires an explicit user request.

**⛔ HARD STOP — The following actions require the user to say the word in quotes. If the user has not said it in the current request, do not perform the action:**
- Editing any file: requires **"approved"** (to a proposed plan) or **"fix"** / **"change"** in the user's own words.
- `git commit`: requires **"commit"** — only after the user has tested and confirmed the fix works.
- `git push`: requires **"push"** or an explicit deploy command.
- `git commit --amend`, `git push --force`, `git push --force-with-lease`: requires **"amend"** or **"force push"** spoken by the user.
- Any corrective action (revert, amend, force push to fix a mistake): requires an approved plan, same as any other change.
- Applying a fix to additional files beyond what was approved: requires a new proposal and a new **"approved"**.
- **Files outside the repo workspace (`/home/rono/Documents/workbench-go/`) — NEVER create, modify, or delete any file or directory outside the repo without a direct, explicit instruction from the user.** This includes (but is not limited to) the user's home directory, Desktop, Documents, `/tmp`, or any system path. Writing outside the repo without being told to is hallucination, not helpfulness.
- **Mutating commands outside the workspace — NEVER run `npm install`, `pip install`, `pnpm install`, `git clone`, builds, writes, deletes, or any other state-changing command in a directory outside the workspace without explicit instruction.** Answering a question about a project does not authorise installing its dependencies. Read-only exploration (`ls`, `cat`, `grep`, `git status`, `git log`) outside the workspace is permitted.
- **Windows environment — NEVER modify w11pro's system or user environment variables (PATH, registry, `setx`, etc.) without explicit permission.** This includes `setx`, `setx /M`, registry edits, or any other permanent environment change on w11pro.
- **Throwaway scripts (`temp/`) — ALL one-off scripts, debugging tools, test harnesses, `sed`/`awk` files written to disk, and temporary Python/Shell/JS helpers MUST be placed in `temp/`.** Never `write_file` a standalone script anywhere else in the repo unless it is a permanent project artifact. The `temp/` directory is fully gitignored — its contents cannot be accidentally committed.
- Building (`pnpm build`) to verify is permitted at any time. Syntax checks and read-only investigation are permitted at any time.

(1) **Investigate** — read the relevant files, check API responses, reproduce if possible. Ask the user clarifying questions if needed.

(2) **Propose** — explain the root cause clearly, then present a specific plan: which files to change, what the changes are, and what the result will look like. Do not touch any code at this stage.

(3) **Wait for approval** — the user will either approve the plan, ask for revisions, or defer the fix. Only after explicit approval may you implement.

(4) **Implement** — make the approved changes on x360. Compile-check or syntax-check them (e.g. `tsc` for frontend, `python3 -c "compile(...)"` for Python). **DO NOT commit to git.** Leave changes in the working tree.

(5) **Verify locally** — confirm the change compiles / passes syntax check on x360. If the development backend/frontend is running on x360, test the fix locally if possible.

(6) **Test** — the user tests the fix on the x360 development site. Only after the user confirms the bug is resolved may you proceed to step 7. If testing reveals the fix is incomplete, return to step 2.

(7) **Commit** — commit the fix to the git repository on x360. Do not push to GitHub.

(8) **Deploy to a514** — a separate step requiring an explicit user request. Never mention or suggest deployment until the user asks for it. Deployment is not part of the bug fix workflow by default.

The same workflow applies to feature requests, design questions, and any work the user has not explicitly authorised. A bug report without "fix it" is an investigation request, not an implementation order. **If you discover an additional bug or needed fix while implementing an approved change, STOP and report the finding. An approval to fix X is not approval to fix Y, even if Y is discovered during X.**

**Rule — Post-Compaction**: After context compaction, re-read the rules sections of this file before acting on the next turn. The Compaction Relay captures session progress and key decisions, not standing operational rules. Do not proceed on memory of the rules alone — re-establish them from this file.

**Rule — Scope Creep**: When a task is complete, do not apply the same changes to other files or pages without explicit confirmation. If you think a completed fix would also benefit another part of the codebase, ask — do not assume. "The same pattern exists elsewhere" is not authorisation to touch those files. Ask first.

**Rule — No Silent Simplification**: Never silently simplify, strip down, or reduce functionality of anything you edit. If you believe a simplification would improve the project or your behavior, propose the change and seek explicit approval before implementing it. Overwriting a detailed artifact with a simplified version without asking is a disaster in the making.

**Rule — No Silent Enhancement**: Never silently add functionality, defaults, fallbacks, or side effects beyond what was requested. This includes "convenience" behaviour — hidden fallbacks (e.g. reading an API key or config from the environment), magic defaults, automatic retries, and secondary actions — that changes the specified contract, cost, identity, or error behaviour. If you believe an addition would improve the work, propose it and seek explicit approval before implementing it. "Making it work" or "making testing easier" is not authorisation to add behaviour the running system will inherit.

**Rule — No Test Shortcuts**: Never change a test's requirements, skip a test, or downgrade a type/assertion just to make a build or test pass. If a test needs a dependency, add it. If a type mapping doesn't work with the current setup, fix the setup — do not change the mapping to a simpler type. Shortcuts that pass today mask real problems whose impact is far higher when discovered later. Not a preference — a hard rule. Test integrity is production integrity.

**Rule — Stored Command Integrity**: Execute every stored command defined in this file exactly as written. Do not optimise, streamline, or deviate from the documented steps without proposing the change and receiving explicit approval. The documented process is the specification; a shortcut that produces an end state you believe is equivalent is not the same as following the process, because the process guarantees things the shortcut does not. If you think a stored command can be improved, propose the change — do not implement it silently.

**Rule — Corrective Actions Require Approval**: If you discover you've made an unauthorized change (scope creep, unapproved edit, wrong file, amended commit, force push, etc.), STOP and report what happened. Do not unilaterally revert, amend commits, force push, or otherwise try to "fix" it yourself. Propose the corrective action and wait for approval — the same as any other change. The urge to clean up quickly is not an exception.

**Rule — Read Before Full-File Write**: Before using `write_file` to replace any existing file, first `read_file` it. Check the current size, structure, and content. A full-file replacement that silently removes sections is exactly the kind of silent simplification this project's rules already prohibit.

**Rule — No Sed, No Awk**: Avoid using `sed` or `awk` for source-code edits — they make too many mistakes. Use `edit_file`, `apply_patch`, or a Python script with explicit search/replace instead. Pattern-based substitutions on code almost always break something.

**Rule — Report, Don't Extend**: When the user asks you to check, verify, inspect, or report on something, that report is the full deliverable. Do not take corrective or follow-up action unless explicitly part of the request. Tell the user what you found, then stop. A report that silently includes unapproved fixes is not a report — it's a report with side effects.

**Rule — Exact Match Python**: Every Go endpoint must produce exactly the same logic, response shape, and behavior as its Python counterpart. Before implementing, produce a side-by-side mapping: Python file:line → Go equivalent. Key requirements:
- Same route paths and HTTP methods
- Same request body fields and validation
- Same database queries (tables, columns, WHERE clauses, ordering)
- Same response JSON keys, types, and nesting
- Same HTTP status codes for each error path
- Same auth checks (middleware vs manual, owner vs shared)
- Any deviation from Python must be explicitly flagged with `// DIFFERENT: reason` in the Go code

Before claiming an endpoint is complete, re-read the full Python source and verify every line has a Go equivalent or an explicit deferral note.

**Rule — Proposal/Implementation Separation**: Proposals and implementations must be in separate turns. If you have not yet received explicit approval for a plan, do not include code edits in the same turn as the proposal. Describe the fix, ask for approval, and only implement after approval is granted. This forces the stop at the turn boundary — you cannot propose and implement in one breath.

**Rule**: Any database schema change — including Prisma schema modifications, new models, new fields, migrations, column renames, or type changes — requires explicit confirmation from the user before writing or executing. Present the proposed change, explain what it does, and wait for approval. Never run `prisma migrate`, `prisma db push`, or `prisma generate` without explicit user confirmation.

**Rule — Pre-Commit Verification**: After making changes on x360, verify before committing. For backend: restart uvicorn (ask the user if remote) and hit the affected endpoint with curl to confirm the expected status code. For frontend: run `pnpm build` — never use `tsc` for frontend verification; `pnpm build` catches issues `tsc` misses. A change that compiles but hasn't been tested is not ready to commit. If you cannot test (backend not running, no auth token, no browser access), state that explicitly and let the user decide whether to commit untested.

**Rule — Artifact Gate**: Before any `edit_file` or `write_file` call, check: is the target an artifact — a file the project says is generated and must not be directly edited? If so, do not touch it. Trace back to the source file and edit that instead. YAML translation files (`frontend/locales/*.yaml`, `backend/locales/*.yaml`), generated CSS, build output, and any file with a documented regeneration process are artifacts. If you already thought "I shouldn't edit this" earlier in the same task, that conclusion is still correct — do not override it.

### 🚫 NO UNAUTHORISED x360 (LOCALHOST) SYSTEM CHANGES — READ THIS FIRST

**CRITICAL — You may NEVER change the x360 development machine's system
configuration without explicit, specific user confirmation.** x360 is the shared
development environment; a system-level change (package install, service restart,
config edit, environment change) can disrupt the user's running sessions and every
other repo that lives on this machine.

- **No `apt-get` / `apt` / `dpkg` installs or removals** — never install or remove
  system packages without explicit confirmation.
- **No `/etc/` or system config edits** — never modify `/etc/`, `/etc/default`,
  `/etc/systemd`, or any other system configuration file without explicit confirmation.
- **No `systemctl` mutating commands** — never start, stop, restart, enable, or
  disable services without explicit confirmation.
- **No environment / PATH changes** — never modify `~/.bashrc`, `~/.profile`,
  `/etc/environment`, `/etc/profile.d/`, or use `update-alternatives` / `update-rc.d`
  without explicit confirmation.
- **No privileged shell** — never run commands as root via `sudo`, `sudo -s`, or
  `ssh root@localhost` to change system state without explicit confirmation.
  `ssh root@localhost` is functionally equivalent to `sudo` but lacks sudo's
  credential-caching timeout and its logging — treat it no more leniently than
  `sudo`, and arguably more strictly.
- **No other root/system mutations** — anything that changes system state outside
  the repo workspace (global installs, service/user/group changes, mounts, kernel
  modules) requires explicit confirmation, regardless of mechanism.

"Explicit confirmation" means the user has stated, in their own words, in the CURRENT
turn, that they want you to perform a specific x360 action. Prior-turn authorisation
does NOT carry forward — every system change requires a fresh, specific instruction in
the current turn. A general discussion is not confirmation.

**Read-only commands** (`apt-cache`, `apt list`, `systemctl status`, `ss`, `df`, `ls`,
`cat`, `grep`, `journalctl`, `git status`, `git log`) are permitted. But a read-only
check does not authorise follow-up mutations.

If you are about to do anything that modifies x360's system configuration and you have
not received explicit confirmation for that specific action, **STOP and ask**.

### 🚫 NO UNAUTHORISED a514 CHANGES — READ THIS FIRST

**CRITICAL — You may NEVER modify anything on a514 without explicit, specific user confirmation. This is the highest-priority rule for production.**

- **No `rsync`** — never push files to a514 with rsync, scp, or any file transfer unless the user explicitly tells you to
- **No `scp` / `sftp`** — same as above; no file transfers of any kind to a514
- **No `apt-get install`** — never install packages on a514 without explicit confirmation
- **No config file edits** — never modify /etc/ files on a514 without explicit confirmation
- **No file creation or deletion** — never create, modify, or delete any file on a514 without explicit confirmation
- **No `systemctl` mutating commands** — never start, stop, restart, enable, or disable services without explicit confirmation

"Explicit confirmation" means the user has stated, in their own words, in the CURRENT turn, that they want you to perform a specific a514 action. Prior-turn authorisation NEVER carries forward to a514 — every rsync, scp, ssh mutation, config edit, or install requires a fresh, specific instruction in the current turn. A general discussion about a514 is not confirmation. The user saying "I need to install Apache on a514" IS explicit confirmation for that specific action and its direct prerequisites — for that turn only.

**Read-only commands** (`systemctl status`, `ss`, `curl` to localhost, `df`, `ls`, `cat`, `grep`, `journalctl`, `git log`, `git status`) are permitted as part of the documented production status check or when explicitly asked. But a read-only check does not authorise follow-up mutations.

If you are about to do anything that modifies a514 state and you have not received explicit confirmation for that specific action, **STOP and ask**.

### "new session"
Read the most recently **modified** `context-transfer-*.md` file in the project root — determined by `ls -lt context-transfer-*.md | head -1`, not by filename timestamp. Context transfers can be updated after creation, so the filename date is not a reliable indicator of recency. After reading the correct file, orient yourself. Read this AGENTS.md again and confirm the rules in it. Also read `~/.codewhale/memory.md` (session memory — declarative facts and preferences) and `~/.codewhale/notes.txt` (workflow lessons and user corrections).

If the context-transfer file records a `Session Log` path, you may retrieve targeted context from it: keyword-search the log (a JSON transcript in `~/.codewhale/sessions/`) for the topics named in the transfer file's In Progress and Next step sections, and read only the matching messages — typically a handful. Do not read the whole log: it is a raw archive, the transfer file is the curated layer, and the log's value is the reasoning behind the decisions the transfer file states. This step is optional and bounded — targeted reads, not wholesale ingestion.

### "generate context transfer" / "save session"
Generate a context-transfer file at the project root: `context-transfer-YYYY-MM-DD-hh:mm:ss.md`. This file is written for an LLM reading it via "new session" — it must be explicit enough that a fresh session can resume work without reverse-engineering intent from file lists or commit logs.

**Execute this instruction exactly as written. Do not optimise, streamline, or deviate without explicit permission.** This includes but is not limited to: updating an older context-transfer file instead of creating a new one, reusing a prior file, altering the filename format, or skipping sections of the template. The date-and-time-stamped filename exists for uniqueness — each invocation produces a new file. If you believe the instruction could be improved, ask before changing it.

The `Session Log` section is required. Record the path exactly as found with `ls -t ~/.codewhale/sessions/*.json | head -1` at save time; do not guess it. Add topic hints where a discussion materially shaped the work (design decisions, bug fixes, user corrections), with approximate message indices.

Structure the file using the Compaction Relay template:

```
### Goal
[One sentence: what the user was trying to accomplish this session]

### Constraints
[What's off-limits, what the user explicitly does NOT want changed, session-specific boundaries like "don't touch a514"]

### Progress
#### Done
[What's complete and verified — landed commits, passing tests, confirmed fixes. Include commit hashes for reference.]

#### In Progress
[What's mid-flight — uncommitted changes with one-line summaries per file. For each modified file, state what changed and why it hasn't been committed yet.]

#### Blocked
[What's stuck, why, and what would unblock it. Include outstanding questions the user hasn't answered yet.]

### Key Decisions
[Architectural choices, design trade-offs, things deferred with rationale. Include a "Conscious Deferrals" subsection for bugs/issues explicitly NOT fixed with the reason why.]

### Outstanding Questions
[Questions posed to the user that are still awaiting an answer. Each as a single line: the question and when it was asked.]

### Session-Specific Preferences
[User preferences expressed DURING this session — e.g. "don't commit without testing", "stop means stop immediately", "don't touch a514". These override or reinforce AGENTS.md rules.]

### Next step
[The single concrete action to take when resuming — one line, specific. Not "fix the bug" but "Run curl against the share endpoint and commit if it returns 200." If the user's last expressed intent was to test something, name what to test and how.]
```

Also include:
- Project overview: stack, key paths, Prisma models (one paragraph)
- Recent commits since the last context transfer (table: hash, subject, date)
- Current git status (`git status --short` output)
- Verification state: what passes (build, backend health, DB accessible)

The file is formatted as structured markdown. Filename includes date and time for uniqueness.



**Rule — Validate, Don't Suppress**: Actions are never disabled (greyed out) to prevent user mistakes. Instead, buttons remain enabled at all times; validation runs on click and failures produce clear, persistent error messages inline. The user should always be able to attempt an action and receive specific feedback about what went wrong, rather than being blocked from trying. A disabled button gives the user no information about what's required; an enabled button with validation gives them a path forward.

**Rule — Pre-Test Confirmation**: After implementing code changes, stop before running any test. Confirm with the user that the environment is ready: backends running on the correct ports, newly built binary deployed, dependencies available, and any required test data (dataset IDs, API keys, tokens) in place. Do not guess the environment state — ask. A clean build is not sufficient evidence to proceed to testing.
