# OpenSpec Agent Guide

## Before Changing Code

- Read `openspec/project.md` and `openspec/config.yaml` for project context.
- Check active work with `openspec list` and existing specs with `openspec list --specs`.
- For non-trivial behavior changes, create or continue a change under `openspec/changes/<change-id>/` before editing implementation files.

## Change Workflow

- Use `/opsx:propose "<idea>"` or `openspec new change "<change-id>"` to start a change.
- Keep proposal, design, tasks, and spec artifacts inside the change directory.
- Requirements should use `SHALL` language and include concrete scenarios.
- Do not implement a change until the required OpenSpec artifacts are present and consistent.
- When implementation is complete, update task checkboxes and archive completed changes with `/opsx:archive` or `openspec archive <change-id>`.

## Project Guardrails

- Keep the runtime dependency-free unless the accepted proposal says otherwise.
- Be conservative with network, routing, firewall, tun/tap, service, UI auth, secret, and proxy exposure behavior.
- Preserve Linux distro compatibility and the existing `install.sh` service-management flow.
- Keep user-facing behavior aligned across code, README, and OpenSpec specs.

## Verification

- Validate OpenSpec artifacts with `openspec validate <change-id>` or the relevant spec validation command.
- Run `python3 -m py_compile vpngate_manager.py proxy_server.py vpn_utils.py` after Python edits.
- Run `bash -n install.sh` after installer edits.
- For operational changes, document or perform smoke checks for service startup, management UI access, VPNGate node fetch, OpenVPN process handling, local proxy behavior, and logs.
