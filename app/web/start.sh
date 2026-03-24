#!/usr/bin/env bash
# Launch 灵台 web dashboard (backend + frontend dev server)
# Usage: ./app/web/start.sh [example]   (default: orchestrator)

set -e
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
EXAMPLE="${1:-orchestrator}"

cd "$REPO_ROOT"
source venv/bin/activate

# Resolve the actual model by tracing through run.py's config logic
python -c "
import json, sys
from pathlib import Path

config_path = Path('app/web/config.json')
if config_path.exists():
    cfg = json.loads(config_path.read_text())
else:
    cfg = {}

# Mirror run.py's resolution logic (including its hardcoded fallback)
provider = cfg.get('provider', 'minimax')
model = cfg.get('model', 'MiniMax-M2.7-highspeed')
base_url = cfg.get('base_url')

# Check provider_defaults override (run.py line 133)
provider_defaults = {}
for pname, pcfg in cfg.get('providers', {}).items():
    if isinstance(pcfg, dict):
        provider_defaults[pname] = {k: v for k, v in pcfg.items() if k != 'api_key_env'}
provider_defaults.setdefault(provider, {})['model'] = model

effective_model = provider_defaults.get(provider, {}).get('model', model)

source = 'config.json' if config_path.exists() and 'model' in cfg else 'run.py fallback'

w = max(len(provider), len(effective_model), len(source), len(base_url or '')) + 12
def row(label, val):
    content = f'{label}{val}'
    print(f'│ {content:<{w-2}}│')
border = '─' * w
print(f'┌{border}┐')
row('Provider: ', provider)
row('Model:    ', effective_model)
row('Source:   ', source)
if base_url:
    row('Base URL: ', base_url)
print(f'└{border}┘')
"

# Start frontend dev server in background
(cd app/web/frontend && npm run dev) &
FRONTEND_PID=$!

# Start backend (foreground — Ctrl+C stops everything)
trap "kill $FRONTEND_PID 2>/dev/null; exit" INT TERM
python -m app.web "$EXAMPLE"
