# Quick actions and prompt templates

Open **Templates** in the sidebar, alongside Skills and Usage, to select and
manage templates. The library groups built-in and custom templates. **Use template**
shows context inputs beside the instruction preview; **Template content** shows
the source with highlighted variables. Create templates with the library header
button, duplicate or edit from the detail header, and delete from its overflow
menu. The bottom bar keeps the insertion target and action visible. The page previews the expanded instruction and names the task
that will receive it. Inserting returns to that task and preserves its draft.
When no editable conversation is selected, insertion targets a new task. Without
a selected project, templates can still be managed but insertion is disabled.

The **Templates** button beside the composer opens a small anchored picker.
Search by name, navigate with Up/Down and choose with Enter (or click a row).
If all fields are available, the expanded template is inserted immediately.
Otherwise, the picker asks only for the required context and parameters; the
full preview can be expanded before insertion. Escape or an outside click closes
the picker. It never sends the message. Template management stays in the sidebar. Select text in the workspace before
opening it, or paste context into the dialog. Textarea selections, browser text
selections (including rendered messages and diffs), and the open terminal's
selection are supported.

Choose **Diagnose error**, **Generate tests**, **Review changes**, or a saved
template. Complete any required parameters, inspect the full instruction preview,
and choose **Insert into composer**. Existing draft text is preserved. Insertion
does not execute an agent: use the existing Run, Queue, Continue, or Steer controls.
The current harness, workflow, attachments and permission settings still apply.
The diagnose and review instructions ask for analysis without edits; these are
prompt instructions, not a separate enforced sandbox.

## Custom templates

In the sidebar **Templates** page, use **New template**, or **Duplicate & edit**
on a built-in action. Custom templates
can be renamed, edited and deleted. A template may contain Markdown; its body is
inserted as text, not evaluated as code.

| Token | Value |
| --- | --- |
| `{{selection}}` | The editable context captured for this action; required when used |
| `{{project}}` | Current project name |
| `{{path}}` | Project directory, or the remote workspace root |
| `{{task}}` | Current task title; empty for a new task |
| `{{date}}` | Local date captured when the dialog opens |
| `{{arg:Focus}}` | A required input named Focus |
| `{{template:custom-ID}}` | Another saved template; copy its reference from its editor |

Nested templates share one parameter form; repeated parameter names appear once.
References resolve by stable ID, so renaming a template does not break references.
Deleting a referenced template makes the dependent template report an error.
Cycles, unknown tokens and missing references are reported before insertion.
Template depth is limited to eight, expansion to 1,000 tokens and 128,000 output
characters. Each stored template allows 32,000 characters, and the library allows
100 templates. Context and parameter values are inserted literally: token-like
text inside a log or code sample is never expanded again.

## Local data

Custom templates are stored in the desktop WebView's local storage under
`onecatch.promptTemplates.v1`. They survive reloads but are not currently stored
in project files, synchronized across devices, or included in diagnostic exports.
Deleting WebView application data removes this library.

Only template IDs, names and unexpanded bodies are saved. Captured selection,
parameter values and generated previews are held in memory for the dialog.
Corrupt or unsupported saved data disables editing rather than overwriting the
library. Storage failures and conflicting edits from another window are reported.

## Implementation and verification

`frontend/src/app/promptActions.js` owns the shared action catalog, bounded template
expansion, library validation and selection helpers. `PromptActions.jsx` is the
shared UI used by both composers. It does not introduce an alternate agent runner
or modify task execution permissions.

This implementation was authored independently for OneCatch, using the product
ideas discussed during comparison with Tinycast; it does not incorporate Tinycast
source code.

Run `npm test` and `npm run build` from `frontend/`. The template-engine tests cover
nested arguments, literal input, recursion and size limits, invalid storage,
bilingual built-ins and preservation of existing drafts. For UI checks, select
text before opening an action, verify required parameters block insertion, save a
template and reload, then insert into both a new task and an existing conversation.
