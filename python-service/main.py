from fastapi import FastAPI, UploadFile, HTTPException
from processors.pdf import parse_pdf
from processors.excel import parse_excel

app = FastAPI(title="audit-python-service")

@app.get("/health")
def health():
    return {"status": "ok"}

@app.post("/parse")
async def parse_document(file: UploadFile):
    content = await file.read()
    name = file.filename or ""

    if name.endswith(".pdf"):
        result = parse_pdf(content)
    elif name.endswith((".xlsx", ".xls")):
        result = parse_excel(content)
    else:
        raise HTTPException(status_code=415, detail="unsupported file type")

    return {
        "filename": name,
        "pages":    result.get("pages"),
        "text":     result.get("text"),
        "tables":   result.get("tables", []),
    }
