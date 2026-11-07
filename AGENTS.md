# Agent access rules

- Do not open, read, summarize, copy, print, index, or expose `.env` or any `.env*` file except committed templates such as `.env.example`.
- If a task appears to require values from `.env`, ask the user to provide the specific non-secret value or to run the command locally instead.
- Do not request, store, or transmit secrets, tokens, passwords, API keys, session cookies, private keys, or connection strings.
- Use `.env.example` for configuration names and expected structure.