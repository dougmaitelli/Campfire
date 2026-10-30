Synthetic Egg compatibility fixtures
===================================

These fixtures are authored for Campfire tests. They follow the publicly
documented Pterodactyl/Pelican export shapes without including upstream installer
scripts or game downloads. Images and commands are representative metadata, not
verified game recipes. The test suite does not pull these images or execute the
scripts. See deploy/EGG-COMPATIBILITY.txt for the support matrix and upstream
format references.

native.json       Native schema 1 regression fixture.
ptdl-v1.json      Legacy single image and JSON-encoded config objects.
ptdl-v2.json      Java-style startup, memory/port templates, ash installer,
                  numeric limits, regex and panel feature warnings.
plcn-v1.json      Legacy startup, string bounds and one-item readiness array.
plcn-v2.json      Object config and newer namespace aliases.
plcn-v3.json      Explicit startup selection and typed JSON scalar replacements.
plcn-v3.yaml      Native YAML, boolean default and literal installer script.
valheim-bepinex.yaml Pelican empty-object lists, URL validation, hidden variables,
                  Bash startup, and game-managed Valheim backup flags. Uses a
                  synthetic installer in place of upstream downloads.
extended.json     Nested JSON/YAML, arrays and wildcards, INI/text config,
                  missing-file creation, readiness alternatives, regex/ANSI
                  detection and variable stop commands.
unsupported.json Hidden variable, file denylist, PCRE lookahead, unsupported
                  template and custom logs; its YAML config and readiness
                  alternatives are now supported while other blockers remain.
