import io
import pdfplumber

def parse_pdf(content: bytes) -> dict:
    text_parts = []
    tables = []

    with pdfplumber.open(io.BytesIO(content)) as pdf:
        pages = len(pdf.pages)
        for page in pdf.pages:
            if page_text := page.extract_text():
                text_parts.append(page_text)
            for table in page.extract_tables():
                tables.append(table)

    return {
        "pages":  pages,
        "text":   "\n".join(text_parts),
        "tables": tables,
    }
