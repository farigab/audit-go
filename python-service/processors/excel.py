import io
import openpyxl

def parse_excel(content: bytes) -> dict:
    wb = openpyxl.load_workbook(io.BytesIO(content), data_only=True)
    sections = []
    tables = []

    for sheet in wb.worksheets:
        rows = [
            [str(cell.value) if cell.value is not None else "" for cell in row]
            for row in sheet.iter_rows()
        ]
        tables.append({"sheet": sheet.title, "rows": rows})

        if not rows:
            continue

        md_lines = [f"## {sheet.title}\n"]
        header = rows[0]
        data   = rows[1:]

        md_lines.append("| " + " | ".join(header) + " |")
        md_lines.append("| " + " | ".join("---" for _ in header) + " |")
        for row in data:
            md_lines.append("| " + " | ".join(row) + " |")

        sections.append("\n".join(md_lines))

    return {
        "pages":    len(wb.worksheets),
        "text":     "\n\n".join(sections),
        "markdown": "\n\n".join(sections),
        "tables":   tables,
    }
