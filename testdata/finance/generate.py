#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Generador de los casos de referencia de testdata/finance (SPIKE-02).

Simula cada caso con ``decimal.Decimal`` (precisión 50) y redondeo
ROUND_HALF_UP a céntimos, implementa las convenciones de
``docs/calculos.md`` y contrasta la cuota de cada tramo con
``numpy_financial.pmt``. Si los dos métodos discrepan en más de 0.01,
falla de forma ruidosa y NO escribe ningún caso.

Los casos 6 a 8 usan valores REALES del Euríbor a 1 año publicados por
el Banco de España (meses de noviembre de 2019 a 2025). Antes de
calcular nada, el generador verifica cada valor contra la captura local
``testdata/bde/listaSeries-euribor-1y-MAX.json`` (serie D_1NBAF472);
si un mes o un valor no coincide con la captura del Banco, aborta sin
escribir nada.

Este generador NO es código de producto: documenta el origen externo de
los casos de referencia. Ningún caso se edita a mano para que un test
pase (docs/ARRANQUE.md §9): si un caso discrepa, se para y se pregunta.

Ejecución:
    python testdata/finance/generate.py

Requiere: numpy y numpy-financial (el resto es biblioteca estándar).
Determinista: dos ejecuciones producen archivos idénticos byte a byte
(fecha de generación fija, sin marcas de tiempo volátiles).
"""

from __future__ import annotations

import hashlib
import json
import platform
import sys
from decimal import Decimal, ROUND_HALF_UP, getcontext
from pathlib import Path
from typing import Optional

import numpy
import numpy_financial

getcontext().prec = 50

CENT = Decimal("0.01")
ONE = Decimal(1)
ZERO = Decimal("0.00")
CROSSCHECK_TOLERANCE = Decimal("0.01")
GENERATED_AT = "2026-10-28"  # fecha fija del spike: nada de timestamps volátiles
OUT_DIR = Path(__file__).resolve().parent
BDE_FIXTURE = OUT_DIR.parent / "bde" / "listaSeries-euribor-1y-MAX.json"
EURIBOR_SERIES = "D_1NBAF472"
INTEREST_CONVENTION = "monthly-nominal"
PAYMENT_ROUNDING = "half-up-cents"


# ---------------------------------------------------------------------------
# Primitivas exactas (convenciones de docs/calculos.md)
# ---------------------------------------------------------------------------

def q2(x: Decimal) -> Decimal:
    """Redondeo half-up a céntimos."""
    return x.quantize(CENT, rounding=ROUND_HALF_UP)


def D(value) -> Decimal:
    return Decimal(str(value))


def monthly_rate(tin_pct: Decimal) -> Decimal:
    """Convención 1: tipo mensual nominal i = TIN / 100 / 12."""
    return tin_pct / Decimal(100) / Decimal(12)


def raw_payment(principal: Decimal, i: Decimal, months: int) -> Decimal:
    """Convención 2: cuota teórica sin redondear.

    C = P · i / (1 − (1 + i)^−n);  con i = 0, C = P / n.
    """
    if months <= 0:
        raise ValueError("months debe ser positivo")
    if i == 0:
        return principal / Decimal(months)
    return principal * i / (ONE - (ONE + i) ** (-months))


def payment(principal: Decimal, i: Decimal, months: int) -> Decimal:
    """Convención 3: la cuota se redondea half-up a céntimos una sola vez."""
    return q2(raw_payment(principal, i, months))


def round_rate_step(tin: Decimal, step: Optional[Decimal]) -> Decimal:
    """Convención 5: redondeo de escritura al múltiplo de `step` más próximo
    (half-up). Con step=None devuelve el tipo sin tocar."""
    if step is None:
        return tin
    multiples = (tin / step).quantize(Decimal("1"), rounding=ROUND_HALF_UP)
    return multiples * step


def fmt_rate(x: Decimal) -> str:
    return str(x)


def fmt_money(x: Decimal) -> str:
    return str(q2(x))


# ---------------------------------------------------------------------------
# Contraste con el método independiente (numpy-financial)
# ---------------------------------------------------------------------------

def fail(message: str) -> None:
    print(f"ERROR: {message}", file=sys.stderr)
    print(
        "Los dos métodos independientes discrepan: NO se escribe ningún caso.",
        file=sys.stderr,
    )
    sys.exit(2)


def crosscheck_payment(label: str, principal: Decimal, i: Decimal,
                       months: int, pay_raw: Decimal) -> Decimal:
    """Compara la cuota exacta (sin redondear) con numpy_financial.pmt."""
    npf_pay = float(numpy_financial.pmt(float(i), months, -float(principal)))
    npf_pay = Decimal(repr(npf_pay))
    diff = abs(npf_pay - pay_raw)
    if diff > CROSSCHECK_TOLERANCE:
        fail(
            f"{label}: cuota Decimal={pay_raw} vs numpy_financial.pmt={npf_pay} "
            f"(diferencia {diff} > {CROSSCHECK_TOLERANCE})"
        )
    return npf_pay


def crosscheck_reduce_term(label: str, principal: Decimal, i: Decimal,
                           n_after: int, kept_payment: Decimal) -> None:
    """reduce-term: comprueba con numpy-financial que n_after es el plazo
    mínimo que cierra el préstamo con la cuota mantenida."""
    npf_n = float(numpy_financial.pmt(float(i), n_after, -float(principal)))
    npf_prev = float(numpy_financial.pmt(float(i), n_after - 1, -float(principal)))
    ok_min = npf_n <= float(kept_payment) + 0.01
    ok_prev = npf_prev > float(kept_payment) - 0.01
    if not (ok_min and ok_prev):
        fail(
            f"{label}: plazo {n_after} no es el mínimo para la cuota "
            f"{kept_payment} (pmt({n_after})={npf_n}, pmt({n_after - 1})={npf_prev})"
        )


# ---------------------------------------------------------------------------
# Procedencia de los índices reales (captura del Banco de España)
# ---------------------------------------------------------------------------

def load_bde_fixture() -> dict:
    """Carga la captura local del Euríbor a 1 año del Banco de España.

    Devuelve {"YYYY-MM": Decimal(valor)} a partir de la serie
    D_1NBAF472. Las fechas vienen en formato ISO con hora; se agrupan por
    mes (YYYY-MM).
    """
    data = json.loads(BDE_FIXTURE.read_text(encoding="utf-8"))
    series = next((s for s in data if s.get("serie") == EURIBOR_SERIES), None)
    if series is None:
        fail(f"la serie {EURIBOR_SERIES} no está en {BDE_FIXTURE}")
    dates = series["fechas"]
    values = series["valores"]
    if len(dates) != len(values):
        fail(f"fechas y valores desalineados en {BDE_FIXTURE}")
    return {date[:7]: Decimal(str(value)) for date, value in zip(dates, values)}


def verify_published_indices(case_id: str, index_entries: list) -> None:
    """Verifica cada índice declarado contra la captura del Banco de España.

    Si el mes de referencia no está en la captura, o el valor no coincide
    exactamente con el publicado, aborta sin escribir ningún caso: la
    procedencia tiene que ser auditable contra los datos del propio Banco.
    """
    fixture = load_bde_fixture()
    for entry in index_entries:
        month = entry["reference_month"]
        declared = D(entry["value"])
        if month not in fixture:
            fail(
                f"{case_id}: el mes de referencia {month} no existe en la "
                f"captura del Banco de España ({BDE_FIXTURE}, serie "
                f"{EURIBOR_SERIES}); NO se escribe ningún caso"
            )
        published = fixture[month]
        if published != declared:
            fail(
                f"{case_id}: {month} declara índice {declared} pero la captura "
                f"del Banco de España ({BDE_FIXTURE}, serie {EURIBOR_SERIES}) "
                f"publica {published}; NO se escribe ningún caso"
            )
        if entry.get("status") != "published" or entry.get("series") != EURIBOR_SERIES:
            fail(
                f"{case_id}: la entrada de {month} no está marcada como "
                f"'published' de la serie {EURIBOR_SERIES}"
            )


def euribor_entries(months_values: list) -> list:
    """Construye las entradas de índice con procedencia completa a partir
    de pares (mes de referencia, valor publicado)."""
    return [
        {
            "revision": position,
            "reference_month": month,
            "value": value,
            "status": "published",
            "series": EURIBOR_SERIES,
        }
        for position, (month, value) in enumerate(months_values, start=1)
    ]


# ---------------------------------------------------------------------------
# Simulación del cuadro (convenciones 3, 4 y 6)
# ---------------------------------------------------------------------------

def simulate_segment(P: Decimal, i: Decimal, pay: Decimal,
                     max_months: int) -> list:
    """Filas de un tramo de tipo constante.

    Cada fila es (mes, cuota, interés, amortización, capital pendiente tras
    la cuota). Convención 4: interés = q2(P·i) antes de amortizar;
    amortización = cuota − interés. Convención 3: la última cuota absorbe
    el residuo para que el capital final sea exactamente 0.00.
    """
    rows = []
    month = 0
    while P > 0:
        month += 1
        if month > max_months:
            fail(f"la simulación supera los {max_months} meses (cuota demasiado pequeña)")
        interest = q2(P * i)
        amort = pay - interest
        if amort <= 0:
            fail(f"mes {month}: la cuota {pay} no cubre el interés {interest}")
        if amort >= P or month == max_months:
            # Última cuota: absorbe el residuo (convención 3).
            rows.append((month, q2(P + interest), interest, P, ZERO))
            P = ZERO
        else:
            P -= amort
            rows.append((month, pay, interest, amort, P))
    return rows


def simulate_fixed(principal: Decimal, tin_pct: Decimal, term: int):
    """Caso de tipo fijo: contraste + simulación del cuadro completo."""
    i = monthly_rate(tin_pct)
    pay_raw = raw_payment(principal, i, term)
    crosscheck_payment(f"cuota (TIN {tin_pct} %, {term} meses)", principal, i, term, pay_raw)
    pay = q2(pay_raw)
    rows = simulate_segment(principal, i, pay, term)
    return rows, pay


# ---------------------------------------------------------------------------
# Casos con revisiones (mixta, suelo, redondeo de escritura)
# ---------------------------------------------------------------------------

def simulate_with_revisions(principal: Decimal, term: int, initial_rate: str,
                            initial_period: int, revision_every: int,
                            index_entries: list, spread: str,
                            floor: Optional[str], step: Optional[Decimal]):
    """Tramo inicial fijo + revisiones periódicas (convención 5).

    En cada revisión: TIN bruto = índice + diferencial; luego el suelo
    (max) y después el redondeo de escritura. La cuota se recalcula con el
    capital pendiente y los meses restantes.
    """
    i0 = monthly_rate(D(initial_rate))
    pay0_raw = raw_payment(principal, i0, term)
    crosscheck_payment("cuota del tramo inicial", principal, i0, term, pay0_raw)
    pay0 = q2(pay0_raw)

    rows = []
    segments = []
    P = principal
    tin = D(initial_rate)
    pay = pay0
    month = 0
    rev_idx = 0
    next_rev_month = initial_period + 1
    while P > 0:
        if month + 1 >= next_rev_month and rev_idx < len(index_entries):
            remaining = term - month
            entry = index_entries[rev_idx]
            index_value = D(entry["value"])
            tin_raw = index_value + D(spread)
            if floor is not None:
                tin_after_floor = max(tin_raw, D(floor))
            else:
                tin_after_floor = tin_raw
            tin_new = round_rate_step(tin_after_floor, step)
            i_new = monthly_rate(tin_new)
            pay_raw = raw_payment(P, i_new, remaining)
            crosscheck_payment(
                f"revisión {rev_idx + 1} (mes {month + 1})", P, i_new, remaining, pay_raw
            )
            pay = q2(pay_raw)
            segments.append({
                "revision_number": rev_idx + 1,
                "first_month": month + 1,
                "reference_month": entry["reference_month"],
                "index_value": fmt_rate(index_value),
                "index_value_status": entry["status"],
                "index_series": entry["series"],
                "spread": spread,
                "tin_raw": fmt_rate(tin_raw),
                "tin_after_floor": fmt_rate(tin_after_floor),
                "tin_after_rounding": fmt_rate(tin_new),
                "remaining_months": remaining,
                "payment": fmt_money(pay),
            })
            tin = tin_new
            rev_idx += 1
            next_rev_month += revision_every
        month += 1
        i = monthly_rate(tin)
        interest = q2(P * i)
        amort = pay - interest
        if amort <= 0:
            fail(f"mes {month}: la cuota {pay} no cubre el interés {interest}")
        if amort >= P or month == term:
            rows.append((month, q2(P + interest), interest, P, ZERO))
            P = ZERO
        else:
            P -= amort
            rows.append((month, pay, interest, amort, P))
    return rows, pay0, segments


# ---------------------------------------------------------------------------
# Amortización anticipada (convención 5 de docs/calculos.md §amortización)
# ---------------------------------------------------------------------------

def simulate_early_repayment(mode: str):
    """Amortiza 10000.00 tras la cuota 60 de un préstamo como el caso 1.

    reduce-payment: mismo plazo, cuota menor recalculada.
    reduce-term: misma cuota, menos meses.
    Comisión = importe · 0 % = 0.00.
    """
    principal = D("150000.00")
    tin = D("3.10")
    term = 300
    amount = D("10000.00")
    event_after_month = 60
    fee_pct = D("0")

    i = monthly_rate(tin)
    pay0_raw = raw_payment(principal, i, term)
    crosscheck_payment("cuota antes de amortizar", principal, i, term, pay0_raw)
    pay0 = q2(pay0_raw)

    baseline_rows = simulate_segment(principal, i, pay0, term)

    rows = baseline_rows[:event_after_month]
    outstanding_before = rows[-1][4]
    fee = q2(amount * fee_pct / Decimal(100))
    new_principal = outstanding_before - amount - fee
    remaining = term - event_after_month

    if mode == "reduce-payment":
        pay_new_raw = raw_payment(new_principal, i, remaining)
        crosscheck_payment(
            f"cuota recalculada ({mode})", new_principal, i, remaining, pay_new_raw
        )
        pay_new = q2(pay_new_raw)
        rows_new = simulate_segment(new_principal, i, pay_new, remaining)
        term_after = term
    elif mode == "reduce-term":
        pay_new = pay0  # se mantiene la cuota
        rows_new = simulate_segment(new_principal, i, pay_new, remaining)
        term_after = event_after_month + len(rows_new)
        crosscheck_reduce_term(
            f"plazo mínimo ({mode})", new_principal, i, len(rows_new), pay_new
        )
    else:
        fail(f"modo desconocido: {mode}")

    # Las filas del tramo posterior al evento se simulan con numeración
    # local (mes 1 = primer mes tras amortizar): se reenumera al mes global.
    rows_new = [
        (month + event_after_month, pay_m, interest_m, amort_m, outstanding_m)
        for (month, pay_m, interest_m, amort_m, outstanding_m) in rows_new
    ]
    rows = rows + rows_new

    event = {
        "mode": mode,
        "after_month": event_after_month,
        "amount": fmt_money(amount),
        "fee_pct": str(fee_pct),
        "fee": fmt_money(fee),
        "outstanding_before_event": fmt_money(outstanding_before),
        "outstanding_after_event": fmt_money(new_principal),
        "remaining_months_at_event": remaining,
    }
    if mode == "reduce-payment":
        event["payment_after_event"] = fmt_money(pay_new)
        event["term_months_after_event"] = term_after
    else:
        event["payment_after_event"] = fmt_money(pay_new)
        event["term_months_after_event"] = term_after
        event["months_saved"] = term - term_after

    return rows, pay0, event, baseline_rows


# ---------------------------------------------------------------------------
# Montaje de los JSON
# ---------------------------------------------------------------------------

def row_json(row: tuple) -> dict:
    return {
        "month": row[0],
        "payment": fmt_money(row[1]),
        "interest": fmt_money(row[2]),
        "principal_repaid": fmt_money(row[3]),
        "outstanding_after": fmt_money(row[4]),
    }


def expected_common(rows: list) -> dict:
    total_interest = q2(sum((r[2] for r in rows), Decimal(0)))
    total_paid = q2(sum((r[1] for r in rows), Decimal(0)))
    sum_principal = q2(sum((r[3] for r in rows), Decimal(0)))
    return {
        "payment": fmt_money(rows[0][1]),
        "total_interest": fmt_money(total_interest),
        "total_paid": fmt_money(total_paid),
        "first_row": row_json(rows[0]),
        "last_row": row_json(rows[-1]),
        "sum_principal_repaid": fmt_money(sum_principal),
        "final_outstanding": "0.00",
    }


def generator_sha256() -> str:
    return hashlib.sha256(Path(__file__).resolve().read_bytes()).hexdigest()


def fixture_sha256() -> str:
    return hashlib.sha256(BDE_FIXTURE.read_bytes()).hexdigest()


def source_block(methods: list, index_fixture: bool = False) -> dict:
    block = {
        "generator": "testdata/finance/generate.py",
        "generator_sha256": generator_sha256(),
        "generated_at": GENERATED_AT,
        "python_version": platform.python_version(),
        "numpy_version": numpy.__version__,
        "numpy_financial_version": numpy_financial.__version__,
        "methods": methods,
        "interest_convention": INTEREST_CONVENTION,
        "payment_rounding": PAYMENT_ROUNDING,
        "crosscheck_tolerance": "0.01",
    }
    if index_fixture:
        block["index_fixture"] = {
            "path": "testdata/bde/listaSeries-euribor-1y-MAX.json",
            "series": EURIBOR_SERIES,
            "sha256": fixture_sha256(),
            "captured_at": GENERATED_AT,
        }
    return block


METHODS_BASE = [
    "simulación exacta con decimal.Decimal (precisión 50) y redondeo "
    "ROUND_HALF_UP a céntimos",
    "contraste de la cuota de cada tramo con numpy_financial.pmt",
]


def fixed_case(case_id: str, description: str, principal: str, tin: str,
               term: int, inputs_extra: Optional[dict] = None) -> dict:
    rows, _pay = simulate_fixed(D(principal), D(tin), term)
    inputs = {
        "principal": principal,
        "annual_rate_pct": tin,
        "term_months": term,
        "interest_convention": INTEREST_CONVENTION,
        "payment_rounding": PAYMENT_ROUNDING,
    }
    if inputs_extra:
        inputs.update(inputs_extra)
    return {
        "id": case_id,
        "description": description,
        "inputs": inputs,
        "expected": expected_common(rows),
        "source": source_block(METHODS_BASE),
    }


def revision_case(case_id: str, description: str, principal: str, term: int,
                  initial_rate: str, initial_period: int, revision_every: int,
                  index_entries: list, spread: str, floor: Optional[str],
                  rate_rounding: str) -> dict:
    """Caso con revisiones usando índices reales verificados.

    Los meses de referencia y los valores de ``index_entries`` se
    contrastan contra la captura del Banco de España antes de calcular
    nada: si no coinciden, ``verify_published_indices`` aborta sin
    escribir ningún archivo.
    """
    verify_published_indices(case_id, index_entries)
    step = Decimal("0.125") if rate_rounding == "1/8" else None
    rows, pay0, segments = simulate_with_revisions(
        D(principal), term, initial_rate, initial_period, revision_every,
        index_entries, spread, floor, step,
    )
    expected = expected_common(rows)
    expected["initial_payment"] = fmt_money(pay0)
    expected["revisions"] = segments
    expected["first_row_after_first_revision"] = row_json(
        next(r for r in rows if r[0] == initial_period + 1)
    )
    inputs = {
        "principal": principal,
        "term_months": term,
        "loan_start": "2019-01-01",
        "payment_day": 1,
        "initial_rate": initial_rate,
        "initial_period_months": initial_period,
        "revision_every_months": revision_every,
        "reference_offset_months": 2,
        "index_values": index_entries,
        "index_values_status": "published",
        "index_source": (
            "Banco de España, serie " + EURIBOR_SERIES
            + " (Euríbor a 1 año), captura testdata/bde/"
        ),
        "spread": spread,
        "floor_rate": floor,
        "rate_rounding": rate_rounding,
        "interest_convention": INTEREST_CONVENTION,
        "payment_rounding": PAYMENT_ROUNDING,
    }
    methods = list(METHODS_BASE)
    methods.append(
        "verificación de cada mes y valor de índice contra la captura del "
        "Banco de España testdata/bde/listaSeries-euribor-1y-MAX.json "
        "(serie " + EURIBOR_SERIES + ")"
    )
    return {
        "id": case_id,
        "description": description,
        "inputs": inputs,
        "expected": expected,
        "source": source_block(methods, index_fixture=True),
    }


def early_repayment_case(case_id: str, description: str, mode: str) -> dict:
    rows, pay0, event, baseline_rows = simulate_early_repayment(mode)
    expected = expected_common(rows)
    # El capital amortizado anticipadamente no es una fila del cuadro, así
    # que se explicita: cuadro + amortización = capital original.
    principal_total = D("150000.00")
    total_interest = q2(sum((r[2] for r in rows), Decimal(0)))
    schedule_principal = q2(sum((r[3] for r in rows), Decimal(0)))
    expected["total_paid"] = fmt_money(total_interest + principal_total)
    expected["sum_principal_repaid"] = fmt_money(principal_total)
    expected["sum_principal_repaid_schedule_only"] = fmt_money(schedule_principal)
    expected["payment_before_event"] = fmt_money(pay0)
    expected["event"] = event
    expected["row_at_event_month"] = row_json(rows[event["after_month"] - 1])
    expected["first_row_after_event"] = row_json(rows[event["after_month"]])
    expected["total_interest_without_early_repayment"] = fmt_money(
        q2(sum((r[2] for r in baseline_rows), Decimal(0)))
    )
    interest_saved = (
        q2(sum((r[2] for r in baseline_rows), Decimal(0)))
        - q2(sum((r[2] for r in rows), Decimal(0)))
    )
    expected["interest_saved"] = fmt_money(interest_saved)
    if mode == "reduce-term":
        expected["months_saved"] = event["months_saved"]
    inputs = {
        "principal": "150000.00",
        "annual_rate_pct": "3.10",
        "term_months": 300,
        "early_repayment": event,
        "interest_convention": INTEREST_CONVENTION,
        "payment_rounding": PAYMENT_ROUNDING,
    }
    methods = list(METHODS_BASE)
    if mode == "reduce-term":
        methods.append(
            "verificación de que el plazo resultante es el mínimo que cierra "
            "el préstamo con la cuota mantenida (numpy_financial.pmt)"
        )
    return {
        "id": case_id,
        "description": description,
        "inputs": inputs,
        "expected": expected,
        "source": source_block(methods),
    }


def build_all_cases() -> list:
    """Construye los diez casos. Todos los contrastes se ejecutan aquí,
    antes de escribir nada: si algo falla, no se escribe ningún archivo."""
    cases = [
        fixed_case(
            "case-01-fijo-150000-300m",
            "Caso base: tipo fijo 3.10 %, cuota constante, cuadro completo y totales.",
            "150000.00", "3.10", 300,
        ),
        fixed_case(
            "case-02-cero-120000-240m",
            "Rama i = 0: cuota = capital / meses, sin intereses. "
            "Sin división por cero en ningún método.",
            "120000.00", "0.00", 240,
        ),
        fixed_case(
            "case-03-bajo-90000-180m",
            "Tipo bajo (0.50 %): el redondeo a céntimos del interés pesa.",
            "90000.00", "0.50", 180,
        ),
        fixed_case(
            "case-04-corto-60000-60m",
            "Plazo corto y tipo alto (6.25 %): cuotas grandes.",
            "60000.00", "6.25", 60,
        ),
        fixed_case(
            "case-05-feos-183450.75-347m",
            "Importes y plazos feos: cuota con muchos decimales antes de redondear.",
            "183450.75", "2.37", 347,
        ),
        revision_case(
            "case-06-mixta-12m",
            "Mixta: 12 meses al 1.25 % y luego 7 revisiones anuales. "
            "Índices reales del Euríbor a 1 año (noviembre 2019-2025, Banco "
            "de España, serie D_1NBAF472), sin ningún valor hipotético: "
            "incluye el estrés real de 2023-2024 (TIN 4.912).",
            "150000.00", 96, "1.25", 12, 12,
            euribor_entries([
                ("2019-11", "-0.272"), ("2020-11", "-0.481"),
                ("2021-11", "-0.487"), ("2022-11", "2.828"),
                ("2023-11", "4.022"), ("2024-11", "2.506"),
                ("2025-11", "2.217"),
            ]),
            "0.890", None, "none",
        ),
        revision_case(
            "case-07-suelo",
            "Revisión con suelo de 3.000: el suelo muerde en 4 de las 7 "
            "revisiones (0.228, 0.019, 0.013 y 2.717 → 3.000) y no muerde en "
            "3.328, 4.522 y 3.006. Índices reales del Euríbor a 1 año "
            "(noviembre 2019-2025, Banco de España, serie D_1NBAF472).",
            "150000.00", 96, "3.500", 12, 12,
            euribor_entries([
                ("2019-11", "-0.272"), ("2020-11", "-0.481"),
                ("2021-11", "-0.487"), ("2022-11", "2.828"),
                ("2023-11", "4.022"), ("2024-11", "2.506"),
                ("2025-11", "2.217"),
            ]),
            "0.500", "3.000", "none",
        ),
        revision_case(
            "case-08-redondeo-octavo",
            "Redondeo de escritura 1/8 en cada revisión (7 de 7 cambian el "
            "TIN por redondeo: 0.618→0.625, 0.409→0.375, 0.403→0.375, "
            "3.718→3.750, 4.912→4.875, 3.396→3.375, 3.107→3.125). Índices "
            "reales del Euríbor a 1 año (noviembre 2019-2025, Banco de "
            "España, serie D_1NBAF472).",
            "150000.00", 96, "3.100", 12, 12,
            euribor_entries([
                ("2019-11", "-0.272"), ("2020-11", "-0.481"),
                ("2021-11", "-0.487"), ("2022-11", "2.828"),
                ("2023-11", "4.022"), ("2024-11", "2.506"),
                ("2025-11", "2.217"),
            ]),
            "0.890", None, "1/8",
        ),
        early_repayment_case(
            "case-09-amortiza-cuota",
            "Amortización anticipada de 10000.00 tras la cuota 60 en modo "
            "reduce-payment: mismo plazo, cuota menor, comisión 0 %.",
            "reduce-payment",
        ),
        early_repayment_case(
            "case-10-amortiza-plazo",
            "Amortización anticipada de 10000.00 tras la cuota 60 en modo "
            "reduce-term: misma cuota, menos meses, comisión 0 %.",
            "reduce-term",
        ),
    ]
    return cases


def write_json(name: str, obj: dict) -> None:
    path = OUT_DIR / name
    text = json.dumps(obj, indent=2, ensure_ascii=False) + "\n"
    with open(path, "w", encoding="utf-8", newline="\n") as handle:
        handle.write(text)


def main() -> None:
    cases = build_all_cases()
    for case in cases:
        name = f"{case['id']}.json"
        write_json(name, case)
        print(
            f"{name}: cuota {case['expected']['payment']} · "
            f"interés total {case['expected']['total_interest']}"
        )
    print(f"OK: {len(cases)} casos escritos en {OUT_DIR}")


if __name__ == "__main__":
    main()
