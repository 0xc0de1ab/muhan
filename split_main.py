import os

with open('cmd/muhan-server/main.go', 'r') as f:
    lines = f.readlines()

header_lines = []
in_import = False
for line in lines:
    if line.startswith('package '):
        header_lines.append(line)
    elif line.startswith('import ('):
        in_import = True
        header_lines.append(line)
    elif in_import:
        header_lines.append(line)
        if line.startswith(')'):
            in_import = False
    elif line.startswith('import "'):
        header_lines.append(line)

header = "".join(header_lines)

def is_start_of_block(line):
    return line.startswith('func ') or line.startswith('type ') or line.startswith('const ') or line.startswith('var ')

blocks = []
current_block = []
brace_count = 0
in_block = False
paren_count = 0

for i, line in enumerate(lines):
    if not in_block and is_start_of_block(line):
        in_block = True
        current_block = []
        brace_count = 0
        paren_count = 0
    
    if in_block:
        current_block.append(line)
        brace_count += line.count('{') - line.count('}')
        paren_count += line.count('(') - line.count(')')
        
        # End of a block?
        if brace_count == 0 and paren_count == 0:
            # If it's a single line type or const without parens
            if '{' not in "".join(current_block) and '(' not in "".join(current_block):
                pass
            # Or if it's the end of a func/struct
            if line.startswith('}') or line.strip() == ')' or (brace_count == 0 and paren_count == 0 and ('{' not in line and '(' not in line)):
                blocks.append("".join(current_block))
                in_block = False

# Now let's classify blocks
files = {
    'config.go': ['type config ', 'type validationError ', 'func (e validationError) Error()', 'func parseFlags('],
    'ws_proxy.go': ['func startWebSocketProxy(', 'func wsAllowedOrigins(', 'func wsOriginAllowed('],
    'helpers.go': ['func serverSafeRootPath(', 'func serverRemoveFiles(', 'func serverCreatureFlag(', 'func serverCreatureInt(', 'func serverTruthy(', 'func serverUniqueStrings(', 'func serverRemoteHost('],
    'password.go': ['type serverPasswordWorld ', 'type serverPasswordSink ', 'func legacyPasswordHash(', 'func (w serverPasswordWorld)', 'func (s serverPasswordSink)'],
    'suicide.go': ['type serverLowLevelQuitSink ', 'type serverSuicideSink ', 'func (s serverLowLevelQuitSink)', 'func (s serverSuicideSink)', 'func serverSuicidePlayerName', 'func serverSuicidePlayerNames', 'func serverSuicideBankIDs', 'func serverDeathFinalizerPlayerID', 'func serverMarkDeathRewardPlayerDirty'],
    'handlers.go': ['type forceWorldWrapper ', 'type marriageSendWorldWrapper ', 'func serverDispatcher(', 'func serverIsDM(', 'func commandHandlers(', 'func (w *forceWorldWrapper)', 'func (w *marriageSendWorldWrapper)'],
    'login.go': ['const defaultListenAddr', 'const loginNamePrompt', 'const loginNewsWaitPrompt', 'const legacyCreateNewFamilyBroadcast', 'const legacyLoginDMClass', 'const legacyLoginDialinHost', 'const legacyLoginGoldCap', 'const legacyLoginGoldCapMessage', 'type serverLoginManager ', 'func newServerLoginManager(', 'func (m *serverLoginManager)', 'func firstLoginRemoteHost(', 'func loginCommand(', 'func legacyCreatePasswordByteLen(', 'type serverLoginPostLoadSequence ', 'func (s *serverLoginPostLoadSequence)', 'func loginReadLegacyText(', 'func loginRegularFileExists(', 'func loginPlayerFileName(', 'func loginSafePostFileName(', 'func legacyCreateNameRejection(', 'func legacyYes(', 'func legacyCreateClassChoice(', 'func legacyCreateRaceChoice(', 'func legacyCreateDigitChoice(', 'func legacyCreateStatChoices(', 'func legacyApplyCreateRaceBonus(', 'func legacyCreatedPlayerCharacter('],
}

main_blocks = []
file_contents = {k: [header] for k in files.keys()}

for b in blocks:
    assigned = False
    for fname, patterns in files.items():
        for p in patterns:
            if b.startswith(p) or p in b.split('\n')[0]:
                file_contents[fname].append(b)
                assigned = True
                break
        if assigned:
            break
    
    if not assigned:
        main_blocks.append(b)

for fname, content_list in file_contents.items():
    if len(content_list) > 1:
        with open('cmd/muhan-server/' + fname, 'w') as f:
            f.write("\n".join(content_list))

with open('cmd/muhan-server/main.go', 'w') as f:
    f.write(header + "\n\n")
    f.write("\n".join(main_blocks))

