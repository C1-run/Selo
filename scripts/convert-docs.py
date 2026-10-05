#!/usr/bin/env python3
"""Convert Markdown docs to HTML pages."""
import os
import re

DOCS_DIR = "/Users/desmondkam/C1-forge/docs"

CSS = """
<style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { 
        font-family: 'SF Pro Display', 'Geist Sans', -apple-system, BlinkMacSystemFont, 'Helvetica Neue', sans-serif; 
        background: #FBFBFA; 
        color: #111111; 
        line-height: 1.7;
        -webkit-font-smoothing: antialiased;
    }
    .container { max-width: 800px; margin: 0 auto; padding: 80px 24px; }
    a { color: #111111; text-decoration: none; }
    a:hover { text-decoration: underline; }
    h1 { 
        font-family: 'Lyon Text', 'Newsreader', 'Playfair Display', 'Instrument Serif', serif;
        font-size: 2.5rem; 
        font-weight: 400;
        letter-spacing: -0.03em;
        line-height: 1.1;
        color: #111111; 
        margin-bottom: 32px; 
    }
    h2 { 
        font-family: 'Lyon Text', 'Newsreader', 'Playfair Display', 'Instrument Serif', serif;
        font-size: 1.5rem; 
        font-weight: 400;
        letter-spacing: -0.02em;
        color: #111111; 
        margin: 48px 0 16px; 
        padding-bottom: 12px;
        border-bottom: 1px solid #EAEAEA; 
    }
    h3 { 
        font-size: 1.125rem; 
        font-weight: 600;
        color: #111111; 
        margin: 32px 0 12px; 
    }
    h4 { 
        font-size: 0.9375rem; 
        font-weight: 600;
        color: #111111; 
        margin: 24px 0 8px; 
    }
    p { margin-bottom: 16px; color: #787774; font-size: 0.9375rem; }
    code { 
        background: #F7F6F3; 
        padding: 2px 6px; 
        border-radius: 3px; 
        font-family: 'Geist Mono', 'SF Mono', 'JetBrains Mono', monospace; 
        color: #111111; 
        font-size: 0.875em; 
    }
    pre { 
        background: #F7F6F3; 
        padding: 20px; 
        border-radius: 6px; 
        overflow-x: auto; 
        margin: 16px 0; 
        border: 1px solid #EAEAEA; 
    }
    pre code { background: none; padding: 0; color: #111111; }
    ul, ol { margin: 16px 0; padding-left: 24px; color: #787774; }
    li { margin: 8px 0; font-size: 0.9375rem; }
    .back { 
        display: inline-block; 
        margin-bottom: 40px; 
        color: #787774;
        font-size: 0.875rem;
    }
    .back:hover { color: #111111; }
    table { 
        width: 100%; 
        border-collapse: collapse; 
        margin: 16px 0; 
        border: 1px solid #EAEAEA;
        border-radius: 6px;
        overflow: hidden;
    }
    th, td { 
        padding: 12px 16px; 
        text-align: left; 
        border-bottom: 1px solid #EAEAEA; 
    }
    th { 
        color: #111111; 
        background: #F7F6F3;
        font-weight: 600;
        font-size: 0.8125rem;
        text-transform: uppercase;
        letter-spacing: 0.05em;
    }
    td { 
        color: #787774;
        font-size: 0.875rem;
    }
    blockquote { 
        border-left: 3px solid #EAEAEA; 
        padding: 12px 20px; 
        margin: 16px 0; 
        background: #F7F6F3; 
        border-radius: 0 6px 6px 0; 
    }
    blockquote p { color: #787774; margin-bottom: 0; }
    hr { border: none; border-top: 1px solid #EAEAEA; margin: 32px 0; }
    strong { color: #111111; font-weight: 600; }
    em { color: #787774; font-style: italic; }
</style>
"""

def md_to_html(md_content):
    """Simple markdown to HTML converter."""
    lines = md_content.split('\n')
    html_lines = []
    in_code_block = False
    in_table = False
    in_list = False
    list_type = None
    
    for line in lines:
        # Code blocks
        if line.strip().startswith('```'):
            if in_code_block:
                html_lines.append('</code></pre>')
                in_code_block = False
            else:
                lang = line.strip()[3:]
                html_lines.append(f'<pre><code class="language-{lang}">')
                in_code_block = True
            continue
        
        if in_code_block:
            html_lines.append(line.replace('<', '&lt;').replace('>', '&gt;'))
            continue
        
        # Tables
        if '|' in line and line.strip().startswith('|'):
            if not in_table:
                html_lines.append('<table>')
                in_table = True
            cells = [c.strip() for c in line.split('|')[1:-1]]
            if all(set(c) <= set('- :') for c in cells):
                continue  # Skip separator row
            tag = 'th' if in_table and not any(set(c) <= set('- :') for c in cells) else 'td'
            html_cells = ''.join(f'<{tag}>{c}</{tag}>' for c in cells)
            html_lines.append(f'<tr>{html_cells}</tr>')
            continue
        elif in_table:
            html_lines.append('</table>')
            in_table = False
        
        # Lists
        if re.match(r'^[\-\*] ', line):
            if not in_list or list_type != 'ul':
                if in_list:
                    html_lines.append(f'</{list_type}>')
                html_lines.append('<ul>')
                in_list = True
                list_type = 'ul'
            item = re.sub(r'^[\-\*] ', '', line)
            html_lines.append(f'<li>{inline_format(item)}</li>')
            continue
        elif re.match(r'^[0-9]+\. ', line):
            if not in_list or list_type != 'ol':
                if in_list:
                    html_lines.append(f'</{list_type}>')
                html_lines.append('<ol>')
                in_list = True
                list_type = 'ol'
            item = re.sub(r'^[0-9]+\. ', '', line)
            html_lines.append(f'<li>{inline_format(item)}</li>')
            continue
        elif in_list:
            html_lines.append(f'</{list_type}>')
            in_list = False
        
        # Headings
        if line.startswith('#### '):
            html_lines.append(f'<h4>{inline_format(line[5:])}</h4>')
        elif line.startswith('### '):
            html_lines.append(f'<h3>{inline_format(line[4:])}</h3>')
        elif line.startswith('## '):
            html_lines.append(f'<h2>{inline_format(line[3:])}</h2>')
        elif line.startswith('# '):
            html_lines.append(f'<h1>{inline_format(line[2:])}</h1>')
        elif line.startswith('---'):
            html_lines.append('<hr>')
        elif line.startswith('> '):
            html_lines.append(f'<blockquote>{inline_format(line[2:])}</blockquote>')
        elif line.strip() == '':
            html_lines.append('')
        else:
            html_lines.append(f'<p>{inline_format(line)}</p>')
    
    if in_list:
        html_lines.append(f'</{list_type}>')
    if in_table:
        html_lines.append('</table>')
    
    return '\n'.join(html_lines)

def inline_format(text):
    """Format inline markdown elements."""
    text = re.sub(r'\*\*(.*?)\*\*', r'<strong>\1</strong>', text)
    text = re.sub(r'\*(.*?)\*', r'<em>\1</em>', text)
    text = re.sub(r'`([^`]+)`', r'<code>\1</code>', text)
    text = re.sub(r'\[([^\]]+)\]\(([^)]+)\)', r'<a href="\2">\1</a>', text)
    return text

def convert_file(md_path):
    """Convert a markdown file to HTML."""
    with open(md_path, 'r') as f:
        content = f.read()
    
    # Extract title
    title_match = re.search(r'^# (.+)$', content, re.MULTILINE)
    title = title_match.group(1) if title_match else 'Documentation'
    
    # Convert markdown to HTML
    body_html = md_to_html(content)
    
    # Build full HTML
    html = f"""<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{title} - Selo</title>
    {CSS}
</head>
<body>
    <div class="container">
        <a href="../index.html" class="back">&larr; Back to Home</a>
        {body_html}
    </div>
</body>
</html>"""
    
    # Write HTML file
    html_path = md_path.replace('.md', '.html')
    with open(html_path, 'w') as f:
        f.write(html)
    
    print(f"Converted: {md_path} -> {html_path}")

# Convert all README.md files
for root, dirs, files in os.walk(DOCS_DIR):
    for file in files:
        if file == 'README.md':
            md_path = os.path.join(root, file)
            convert_file(md_path)

# Convert demo.md
demo_path = os.path.join(DOCS_DIR, 'demo.md')
if os.path.exists(demo_path):
    convert_file(demo_path)

print("Done!")
