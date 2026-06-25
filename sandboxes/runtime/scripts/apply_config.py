"""Read config from stdin, apply sane defaults, print updated config to stdout."""
import json
import sys

d = json.load(sys.stdin)

system_prompt = d.setdefault('system_prompt', {})
dynamic_segments = system_prompt.setdefault('dynamic_segments', [])
existing_dynamic = {segment.get('type') for segment in dynamic_segments}
for segment in [
    {
        'type': 'skill_catalog',
        'config': {
            'title': 'Available skills (load when relevant)',
            'preamble': "Before using tools for a task, check this list and call skill_view(name) when a skill matches the user's request. Do not load unrelated skills.",
            'item_template': '- {name}: {description}',
        },
    },
    {
        'type': 'mcp_tools',
        'config': {
            'title': 'Available MCP tools (use directly)',
            'item_template': '- {name}',
        },
    },
]:
    if segment['type'] not in existing_dynamic:
        dynamic_segments.append(segment)

if 'context' not in d:
    d['context'] = {}
comp = d['context'].get('compaction')
if comp is None:
    d['context']['compaction'] = {
        'enabled': True,
        'token_threshold': 90000,
        'overlap_event_count': 10,
        'chars_per_token': 4,
        'summarizer_model': d['model'],
    }

print(json.dumps(d))
