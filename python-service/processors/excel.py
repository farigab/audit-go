import io
import openpyxl

def parse_excel(content: bytes) -> dict:
    wb = openpyxl.load_workbook(io.BytesIO(content), data_only=True)
    tables = []

    for sheet in wb.worksheets:
        rows = [[str(cell.value) if cell.value is not None else ""
                 for cell in row]
                for row in sheet.iter_rows()]
        tables.append({"sheet": sheet.title, "rows": rows})

    return {
        "pages":  len(wb.worksheets),
        "text":   "",
        "tables": tables,
    }
