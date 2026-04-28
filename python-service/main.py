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
        "markdown": result.get("markdown"),  # ← campo novo
        "tables":   result.get("tables", []),
    }
