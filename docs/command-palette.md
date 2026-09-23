# Command palette

Open global search with Command+K on macOS or Ctrl+K on Windows/Linux.
Search tasks and projects, or navigate to Templates, Skills, Usage and Settings.
New task and Open folder are also available as commands.

Commands accept Chinese and English keywords regardless of the UI language.
Examples: `mb` or `templates` opens Templates; `quota` or `用量` opens Usage;
`sz` opens Settings. Short Latin abbreviations allow skipped letters, such as
`tmplt`. Multiple words must all match. Matching commands appear before task
and project results while searching.

With an empty query, recent commands appear first within the command group.
Only a bounded list of command IDs is stored locally under
`onecatch.commandHistory.v1`; search queries and task content are not saved.
Unavailable browser storage does not prevent navigation.

Use Up/Down to move, Home/End for the first/last result, and Enter to activate.
Escape, Command/Ctrl+K or clicking outside closes the palette. Tab keeps focus
inside the search input. Numbered task/project shortcuts remain available.

The command catalog and matching logic live in `paletteCommands.js` and have
unit coverage for multilingual matching, ranking and invalid/unavailable storage.
