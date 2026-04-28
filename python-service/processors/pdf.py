import io
import pdfplumber

def parse_pdf(content: bytes) -> dict:
    text_parts = []
    tables = []

    with pdfplumber.open(io.BytesIO(content)) as pdf:
        pages = len(pdf.pages)
        for i, page in enumerate(pdf.pages, start=1):
            if page_text := page.extract_text():
                text_parts.append(f"## Página {i}\n\n{page_text}")
            for table in page.extract_tables():
                tables.append(table)
                text_parts.append(_table_to_markdown(table))

    return {
        "pages":    pages,
        "text":     "\n\n".join(text_parts),
        "markdown": "\n\n".join(text_parts),
        "tables":   tables,
    }

def _table_to_markdown(table: list[list]) -> str:
    if not table or not table[0]:
        return ""

    header = table[0]
    rows   = table[1:]

    lines = []
    lines.append("| " + " | ".join(str(c or "") for c in header) + " |")
    lines.append("| " + " | ".join("---" for _ in header) + " |")
    for row in rows:
        lines.append("| " + " | ".join(str(c or "") for c in row) + " |")

    return "\n".join(lines)
