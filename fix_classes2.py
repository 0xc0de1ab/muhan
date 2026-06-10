import os
import re

replacements = {
    'legacyClassZoneMaker': 'model.ClassZoneMaker',
    'legacyClassAssassin': 'model.ClassAssassin',
    'legacyClassBarbarian': 'model.ClassBarbarian',
    'legacyClassCleric': 'model.ClassCleric',
    'legacyClassFighter': 'model.ClassFighter',
    'legacyClassMage': 'model.ClassMage',
    'legacyClassPaladin': 'model.ClassPaladin',
    'legacyClassRanger': 'model.ClassRanger',
    'legacyClassThief': 'model.ClassThief',
    'legacyClassInvincible': 'model.ClassInvincible',
    'legacyClassCaretaker': 'model.ClassCaretaker',
    'legacyClassBulsa': 'model.ClassBulsa',
    'legacyClassSubDM': 'model.ClassSubDM',
    'legacyClassDM': 'model.ClassDM',

    'healClassAssassin': 'model.ClassAssassin',
    'healClassBarbarian': 'model.ClassBarbarian',
    'healClassCleric': 'model.ClassCleric',
    'healClassFighter': 'model.ClassFighter',
    'healClassMage': 'model.ClassMage',
    'healClassPaladin': 'model.ClassPaladin',
    'healClassRanger': 'model.ClassRanger',
    'healClassThief': 'model.ClassThief',
    'healClassInvincible': 'model.ClassInvincible',
    'healClassCaretaker': 'model.ClassCaretaker',
    'healClassBulsa': 'model.ClassBulsa',
    'healClassSubDM': 'model.ClassSubDM',
    'healClassDM': 'model.ClassDM',

    'stateLegacyClassSubDM': 'model.ClassSubDM',
    'stateLegacyClassDM': 'model.ClassDM',
    'stateLegacyClassFighter': 'model.ClassFighter',
    'stateLegacyClassInvincible': 'model.ClassInvincible',
    'stateLegacyClassCaretaker': 'model.ClassCaretaker',
    'stateLegacyClassBulsa': 'model.ClassBulsa',
    'stateLegacyClassMage': 'model.ClassMage',
    'stateLegacyClassAssassin': 'model.ClassAssassin',
    'stateLegacyClassBarbarian': 'model.ClassBarbarian',
    'stateLegacyClassCleric': 'model.ClassCleric',
    'stateLegacyClassPaladin': 'model.ClassPaladin',
    'stateLegacyClassRanger': 'model.ClassRanger',
    'stateLegacyClassThief': 'model.ClassThief',
    'appraiseClassAssassin': 'model.ClassAssassin',
    'legacyBoardDMClass': 'model.ClassDM',
    'legacyBoardSubDMClass': 'model.ClassSubDM',
}

def process_file(filepath):
    if 'model/class.go' in filepath or 'fix_classes' in filepath:
        return

    with open(filepath, 'r') as f:
        lines = f.readlines()

    new_lines = []
    modified = False

    for line in lines:
        # Skip constant definitions
        if re.match(r'^\s*(legacyClass|stateLegacyClass|healClass|appraiseClass|legacyBoard)[A-Za-z]+\s*=\s*.*', line):
            modified = True
            continue
        
        new_line = line
        for old, new in replacements.items():
            # Use negative lookbehind and lookahead to avoid partial matches
            # e.g., don't match legacyClassInvincibleForServerTest
            new_line = re.sub(r'(?<![a-zA-Z0-9_])' + old + r'(?![a-zA-Z0-9_])', new, new_line)
        
        if new_line != line:
            modified = True
        
        new_lines.append(new_line)

    if modified:
        content = "".join(new_lines)
        if 'model.' in content and '"muhan/internal/world/model"' not in content:
            content = content.replace('import (', 'import (\n\t"muhan/internal/world/model"\n', 1)
        
        with open(filepath, 'w') as f:
            f.write(content)

for root, dirs, files in os.walk('.'):
    for name in files:
        if name.endswith('.go'):
            process_file(os.path.join(root, name))
