# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""scripts/compliance/import-frameworks.py parses a downloaded workbook with a size cap (S-TRUE-WIRE Step 361).

The bandit B314/B405 nosec on its ElementTree calls rests on two controls: every XML member of
the .xlsx is refused when it inflates past MAX_BYTES (a zip bomb inside a 64 MiB download), and
expat is new enough to cap entity amplification. These tests build a workbook in memory."""
import importlib.util
import io
import pathlib
import zipfile

import pytest

ROOT = pathlib.Path(__file__).resolve().parents[2]
M = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
R = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"


@pytest.fixture(scope="module")
def imp():
    spec = importlib.util.spec_from_file_location("import_frameworks", ROOT / "scripts" / "compliance" / "import-frameworks.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def workbook(sheet_xml):
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as z:
        z.writestr("xl/workbook.xml",
                   f'<workbook xmlns="{M}" xmlns:r="{R}"><sheets><sheet name="S" r:id="rId1"/></sheets></workbook>')
        z.writestr("xl/_rels/workbook.xml.rels",
                   '<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>')
        z.writestr("xl/worksheets/sheet1.xml", sheet_xml)
    return buf.getvalue()


def test_parses_a_small_workbook(imp):
    sheet = f'<worksheet xmlns="{M}"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>AC-1</t></is></c></row></sheetData></worksheet>'
    out = imp.xlsx_sheets(workbook(sheet))
    assert "S" in out


def test_refuses_a_member_over_the_cap(imp, monkeypatch):
    monkeypatch.setattr(imp, "MAX_BYTES", 1024)
    sheet = f'<worksheet xmlns="{M}"><sheetData>' + "<row/>" * 1000 + "</sheetData></worksheet>"
    with pytest.raises(SystemExit):
        imp.xlsx_sheets(workbook(sheet))


def test_expat_has_the_amplification_limit():
    import pyexpat
    assert pyexpat.version_info >= (2, 4, 1)
