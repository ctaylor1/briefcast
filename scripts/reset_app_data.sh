#!/usr/bin/env sh
set -eu
# The old monolith reset bypassed worker drain/state recovery and explicit modes.
echo 'Destructive reset is disabled for the split stack. Use the reviewed recovery procedure in docs/whisperx-separation/EXECUTION_PLAN.md.' >&2
exit 1
