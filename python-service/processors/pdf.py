import io

import pdfplumber


def parse_pdf(content: bytes) -> dict:
    text_parts = []
    tables = []

    with pdfplumber.open(io.BytesIO(content)) as pdf:
        pages = len(pdf.pages)
        for i, page in enumerate(pdf.pages, start=1):
            if page_text := page.extract_text():
                text_parts.append(f"## Pagina {i}\n\n{page_text}")
            for table in page.extract_tables():
                rows = _normalize_table(table)
                tables.append({"page": i, "rows": rows})
                text_parts.append(_table_to_markdown(rows))

    markdown = "\n\n".join(text_parts)
    return {
        "pages": pages,
        "text": markdown,
        "markdown": markdown,
        "tables": tables,
    }


def _normalize_table(table: list[list]) -> list[list[str]]:
    return [[str(c) if c is not None else "" for c in row] for row in table]


def _table_to_markdown(table: list[list[str]]) -> str:
    if not table or not table[0]:
        return ""

    header = table[0]
    rows = table[1:]

    lines = []
    lines.append("| " + " | ".join(header) + " |")
    lines.append("| " + " | ".join("---" for _ in header) + " |")
    for row in rows:
        lines.append("| " + " | ".join(row) + " |")

    return "\n".join(lines)
