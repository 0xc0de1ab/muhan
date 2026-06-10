import re

with open('/workspace/muhan/cmd/muhan-server/main.go', 'r') as f:
    content = f.read()

# Replace logger init in run or main
init_slog = """	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
"""

def log_repl(m):
    original = m.group(0)
    level_tag = m.group(1)
    msg = m.group(2)
    args = m.group(3)
    
    level_map = {
        'ERROR': 'Error',
        'WARN': 'Warn',
        'INFO': 'Info',
        'SECURITY': 'Warn' # fallback for [SECURITY] WARN
    }
    
    if '[SECURITY] WARN' in original:
        level = 'Warn'
        msg = '[SECURITY] ' + msg
    else:
        level = level_map.get(level_tag, 'Info')
        
    if args:
        if '%v' in msg or '%s' in msg or '%d' in msg:
            # We just do slog.Xxx(fmt.Sprintf(...)) for simplicity, or 
            # if the only %v is at the end, we can do slog.Xxx(msg, "err", err)
            pass
        return f'slog.{level}(fmt.Sprintf("{msg}", {args}))'
    else:
        return f'slog.{level}("{msg}")'

# First pass for pattern: log.Printf("[CATEGORY] LEVEL message: %v", args)
# Actually let's just use a simple regex for all log.Printf
def general_repl(m):
    fmt_str = m.group(1)
    args = m.group(2)
    
    # Check if there is a level bracket
    level = 'Info'
    if 'ERROR' in fmt_str:
        level = 'Error'
    elif 'WARN' in fmt_str:
        level = 'Warn'
    
    # Remove the tag from the format string to be cleaner, or keep it.
    if args:
        return f'slog.{level}(fmt.Sprintf({fmt_str}, {args}))'
    else:
        return f'slog.{level}({fmt_str})'

content = re.sub(r'log\.Printf\((.*?)(?:,\s*(.*?))?\)', general_repl, content)

with open('/workspace/muhan/cmd/muhan-server/main.go', 'w') as f:
    f.write(content)

