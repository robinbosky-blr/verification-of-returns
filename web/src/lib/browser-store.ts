import * as XLSX from 'xlsx';
import { unzipSync } from 'fflate';
import type { Case, Import } from './api';

export type Marketplace = 'amazon' | 'flipkart';

export type BrowserAudit = { id: number; action: string; details: unknown; created_at: string };

export type SourceRow = {
  import_id: string;
  kind: 'website' | 'physical' | 'payment';
  order_id: string;
  sku: string;
  date: string;
  updated: string;
  delivered: string;
  status: string;
  reason: string;
  type: string;
  channel: string;
  reference_id: string;
  amount: number;
  source: string;
  line: number;
  imported_at: string;
  occurrence?: number;
};

type ClaimData = Pick<
  Case,
  | 'claim_raised'
  | 'claim_raised_date'
  | 'user_claim_status'
  | 'claim_updated_date'
  | 'amazon_claim_id'
  | 'claim_amount_requested'
  | 'claim_amount_approved'
  | 'claim_rejection_reason'
  | 'claim_notes'
  | 'user_notes'
>;

export type BrowserState = {
  version: 1;
  rows: SourceRow[];
  imports: Import[];
  claims: Record<string, ClaimData>;
  audit: BrowserAudit[];
};

const DB_NAME = 'returnscope-amazon-v1';
const DB_VERSION = 2;
const STORE = 'workspace';
const LEGACY_KEY = 'primary';
const MAX_FILE_SIZE = 100 * 1024 * 1024;

const emptyState = (): BrowserState => ({ version: 1, rows: [], imports: [], claims: {}, audit: [] });

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    if (!globalThis.indexedDB) { reject(new Error('Private browser storage is unavailable. Open the site in Chrome or Edge with site storage enabled.')); return; }
    const request = globalThis.indexedDB.open(DB_NAME, DB_VERSION);
    request.onupgradeneeded = () => {
      if (!request.result.objectStoreNames.contains(STORE)) request.result.createObjectStore(STORE);
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error ?? new Error('Could not open the browser workspace.'));
  });
}

export async function loadBrowserState(marketplace: Marketplace = 'amazon'): Promise<BrowserState> {
  const db = await openDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE, 'readonly');
    const store = tx.objectStore(STORE);
    const request = store.get(marketplace);
    request.onsuccess = () => {
      if (request.result) { resolve(request.result as BrowserState); return; }
      if (marketplace !== 'amazon') { resolve(emptyState()); return; }
      const legacy = store.get(LEGACY_KEY);
      legacy.onsuccess = () => resolve((legacy.result as BrowserState | undefined) ?? emptyState());
      legacy.onerror = () => reject(legacy.error ?? new Error('Could not read the Amazon browser workspace.'));
    };
    request.onerror = () => reject(request.error ?? new Error('Could not read the browser workspace.'));
    tx.oncomplete = () => db.close();
  });
}

export async function saveBrowserState(state: BrowserState, marketplace: Marketplace = 'amazon'): Promise<void> {
  const db = await openDB();
  const persisted = JSON.parse(JSON.stringify(state)) as BrowserState;
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE, 'readwrite');
    tx.objectStore(STORE).put(persisted, marketplace);
    tx.oncomplete = () => { db.close(); resolve(); };
    tx.onerror = () => { db.close(); reject(tx.error ?? new Error('Could not save the browser workspace.')); };
  });
}

const clean = (value: unknown) => String(value ?? '').replace(/[\u200b\ufeff]/g, '').trim();
const normalize = (value: unknown) => clean(value).replace(/\s+/g, ' ').toUpperCase();
const keyOf = (order: string, sku: string) => `${normalize(order)}\u001f${normalize(sku)}`;
const header = (value: unknown) => clean(value).toLowerCase().replace(/[^a-z0-9]/g, '');

const aliases: Record<string, string[]> = {
  order: ['customerorderid', 'orderid', 'amazonorderid', 'merchantorderid', 'orderno', 'ordernumber'],
  sku: ['msku', 'sku', 'sellersku', 'merchantsku'],
  date: ['returnrequesteddate', 'returnrequestdate', 'returncreationdate', 'returninitiateddate', 'returnrequestedon', 'returncreationtime', 'returncreatedon', 'returndate', 'carrierpickupdate', 'pickupdate', 'receiveddate', 'receivedon', 'dateofreceived', 'physicalreceiveddate', 'physicallyreceiveddate', 'date', 'datetime', 'transactiondate', 'transactionposteddate', 'posteddate', 'postedon'],
  updated: ['lastupdatedon', 'lastupdateddate', 'lastupdated', 'updatedat', 'statusupdatedon'],
  delivered: ['returndelivereddate', 'delivereddate', 'deliveredtosellerdate'],
  status: ['returnstatus', 'shipmentstatus', 'status'],
  type: ['type', 'transactiontype', 'paymenttype'],
  amount: ['total', 'totalinr', 'totalamount', 'amount', 'transactionamount', 'transactiontotal', 'netamount', 'netproceeds', 'nettotal'],
  quantity: ['quantity', 'qty', 'returnquantity'],
  channel: ['fulfilment', 'fulfillment', 'fulfilmentchannel', 'fulfillmentchannel', 'returntype'],
  reason: ['returnreason', 'reason', 'subreason', 'customerreturnreason', 'returnsubreason'],
  reference: ['transactionid', 'amazontransactionid', 'referenceid', 'transactionreference', 'transactionreferenceid', 'settlementid', 'settlementidentifier'],
  approval: ['returnapprovaldate'],
  completed: ['completeddate'],
  out_for_delivery: ['outfordeliverydate'],
  sale: ['saleamountrs'],
  refund: ['refundrs'],
  protection: ['protectionfundrs'],
  marketplace_fee: ['marketplacefeerssumvai', 'marketplacefeers']
};

const amazonSourceAliases: Record<SourceRow['kind'], Partial<Record<string, string[]>>> = {
  website: {
    order: ['customerorderid', 'amazonorderid', 'orderid', 'merchantorderid', 'orderno', 'ordernumber'],
    // Amazon website exports contain both listing SKU and merchant mSKU. Reconciliation uses mSKU.
    sku: ['msku', 'merchantsku', 'sellersku'],
    date: ['returncreationdate', 'returncreatedon', 'returncreationtime', 'carrierpickupdate', 'pickupdate']
  },
  physical: {
    order: ['orderid', 'customerorderid', 'amazonorderid', 'merchantorderid', 'orderno', 'ordernumber'],
    sku: ['sku', 'msku', 'sellersku', 'merchantsku'],
    date: ['returndate', 'receiveddate', 'receivedon', 'dateofreceived', 'physicalreceiveddate', 'physicallyreceiveddate', 'date']
  },
  payment: {
    order: ['orderid', 'amazonorderid', 'customerorderid', 'merchantorderid', 'orderno', 'ordernumber'],
    sku: ['sku', 'msku', 'sellersku', 'merchantsku'],
    date: ['datetime', 'transactiondate', 'transactionposteddate', 'posteddate', 'postedon', 'date']
  }
};

const flipkartSourceAliases: Record<SourceRow['kind'], Partial<Record<string, string[]>>> = {
  website: {
    order: ['orderid', 'orderitemid', 'orderunitid', 'shipmentid', 'returnid'],
    sku: ['sku', 'sellersku', 'merchantsku', 'fsn'],
    date: ['returncreationdate', 'returnrequesteddate', 'returnrequestdate', 'returndate', 'createddate'],
    updated: ['lastupdateddate', 'lastupdatedon', 'statusupdateddate', 'updateddate'],
    approval: ['returnapprovaldate'],
    completed: ['completeddate'],
    out_for_delivery: ['outfordeliverydate'],
    delivered: ['returndelivereddate', 'delivereddatetoseller', 'completeddate'],
    status: ['returnstatus', 'status', 'shipmentstatus'],
    channel: ['returntype', 'deliverytype', 'fulfilmenttype'],
    reason: ['returnreason', 'returnsubreason', 'reason', 'subreason', 'customerreturnreason'],
    reference: ['orderitemid']
  },
  physical: {
    order: ['orderid', 'orderitemid', 'orderunitid', 'shipmentid', 'returnid'],
    sku: ['sku', 'sellersku', 'fsn'],
    date: ['receiveddate', 'physicalreceiveddate', 'returndate', 'date'],
    reference: ['orderitemid']
  },
  payment: {
    order: ['orderid', 'orderitemid', 'orderunitid', 'shipmentid'],
    sku: ['sku', 'sellersku', 'fsn'],
    date: ['paymentdate', 'transactiondate', 'settlementdate', 'date'],
    type: ['transactiontype', 'paymenttype', 'type', 'description'],
    amount: ['amount', 'totalamount', 'netamount', 'settlementvalue', 'total'],
    reference: ['orderitemid', 'transactionid', 'referenceid', 'settlementid', 'claimid'],
    sale: ['saleamountrs'],
    refund: ['refundrs'],
    protection: ['protectionfundrs'],
    marketplace_fee: ['marketplacefeerssumvai', 'marketplacefeers']
  }
};

function columns(row: unknown[], kind: SourceRow['kind'], marketplace: Marketplace): Record<string, number> {
  const result: Record<string, number> = {};
  const normalized = row.map(header);
  const sourceAliases = marketplace === 'flipkart' ? flipkartSourceAliases : amazonSourceAliases;
  for (const field of Object.keys(aliases)) {
    for (const candidate of sourceAliases[kind][field] ?? aliases[field]) {
      const index = normalized.indexOf(candidate);
      if (index >= 0) {
        result[field] = index;
        break;
      }
    }
  }
  return result;
}

function validColumns(cols: Record<string, number>, kind: string, marketplace: Marketplace) {
  if (cols.order === undefined) return false;
  if (kind === 'website') {
    if (cols.sku === undefined || cols.status === undefined) return false;
    return marketplace === 'flipkart' ? cols.date !== undefined : cols.updated !== undefined;
  }
  if (kind === 'physical') return marketplace === 'flipkart' || cols.sku !== undefined;
  if (kind === 'payment') {
    if (cols.date === undefined) return false;
    if (marketplace === 'flipkart') return (cols.type !== undefined && cols.amount !== undefined) || cols.sale !== undefined || cols.refund !== undefined || cols.protection !== undefined;
    return cols.type !== undefined && cols.amount !== undefined;
  }
  return true;
}

function isoDate(value: unknown, field: string): string {
  if (value instanceof Date && !Number.isNaN(value.getTime())) return value.toISOString();
  if (typeof value === 'number' && value > 30000 && value < 100000) {
    const parsed = XLSX.SSF.parse_date_code(value);
    if (parsed) return new Date(Date.UTC(parsed.y, parsed.m - 1, parsed.d, parsed.H, parsed.M, Math.floor(parsed.S))).toISOString();
  }
  let s = clean(value).replace(/\s+(IST|UTC|GMT\+05:30)$/i, '');
  if (!s) throw new Error(`${field} is missing`);
  const direct = /^\d{4}-\d{2}-\d{2}/.test(s) ? new Date(s.includes('T') ? s : `${s.replace(' ', 'T')}+05:30`) : null;
  if (direct && !Number.isNaN(direct.getTime())) return direct.toISOString();
  const monthNames: Record<string, number> = { jan: 0, feb: 1, mar: 2, apr: 3, may: 4, jun: 5, jul: 6, aug: 7, sep: 8, oct: 9, nov: 10, dec: 11 };
  let match = s.match(/^(\d{1,2})[-/ ]([A-Za-z]{3,9})[-/ ,](\d{4})(?:\s+(\d{1,2}):(\d{2})(?::(\d{2}))?\s*(AM|PM)?)?$/i);
  if (match) {
    const month = monthNames[match[2].slice(0, 3).toLowerCase()];
    if (month !== undefined) {
      let hour = Number(match[4] || 0);
      const ap = (match[7] || '').toUpperCase();
      if (ap === 'PM' && hour < 12) hour += 12;
      if (ap === 'AM' && hour === 12) hour = 0;
      return new Date(Date.UTC(Number(match[3]), month, Number(match[1]), hour, Number(match[5] || 0), Number(match[6] || 0)) - 19800000).toISOString();
    }
  }
  match = s.match(/^(\d{1,2})[./-](\d{1,2})[./-](\d{2}|\d{4})(?:\s+(\d{1,2}):(\d{2})(?::(\d{2}))?)?$/);
  if (match) {
    let year = Number(match[3]);
    if (year < 100) year += 2000;
    return new Date(Date.UTC(year, Number(match[2]) - 1, Number(match[1]), Number(match[4] || 0), Number(match[5] || 0), Number(match[6] || 0)) - 19800000).toISOString();
  }
  const parsed = new Date(s);
  if (!Number.isNaN(parsed.getTime())) return parsed.toISOString();
  throw new Error(`${field} ${JSON.stringify(s)} is not a recognized date; use DD/MM/YYYY or ISO format`);
}

function monthFromSheet(name: string, fallback: string): string {
  if (/^\d{4}-\d{2}$/.test(name)) return name;
  const names = ['january', 'february', 'march', 'april', 'may', 'june', 'july', 'august', 'september', 'october', 'november', 'december'];
  const lower = name.toLowerCase();
  const index = names.findIndex((m) => lower.includes(m) || lower.includes(m.slice(0, 3)));
  const year = name.match(/20\d{2}/)?.[0] ?? (fallback.slice(0, 4) || String(new Date().getFullYear()));
  if (index >= 0 && year) return `${year}-${String(index + 1).padStart(2, '0')}`;
  if (fallback) return fallback;
  throw new Error(`Physical receipt date is missing in sheet ${JSON.stringify(name)}; select the physical month first.`);
}

function money(value: unknown): number {
  let s = clean(value).replace(/,/g, '').replace(/₹/g, '').replace(/INR/gi, '').replace(/Rs\./gi, '');
  if (!s) return 0;
  if (s.startsWith('(') && s.endsWith(')')) s = `-${s.slice(1, -1)}`;
  const amount = Number(s);
  if (!Number.isFinite(amount)) throw new Error(`Invalid amount ${JSON.stringify(value)}`);
  return Math.round(amount * 100);
}

function parseSheet(data: unknown[][], name: string, marketplace: Marketplace, kind: SourceRow['kind'], coverage: string, importId: string, importedAt: string, optionalSheet = false): SourceRow[] {
  let cols: Record<string, number> | null = null;
  let previousOrder = '';
  const output: SourceRow[] = [];
  for (let index = 0; index < data.length; index++) {
    const cells = data[index] ?? [];
    if (!cells.some((v) => clean(v))) { previousOrder = ''; continue; }
    const possible = columns(cells, kind, marketplace);
    if (validColumns(possible, kind, marketplace)) { cols = possible; previousOrder = ''; continue; }
    if (!cols) {
      if (index >= 49) break;
      continue;
    }
    const get = (field: string) => cols![field] === undefined ? '' : cells[cols![field]];
    let order = normalize(get('order'));
    const sku = normalize(get('sku'));
    if (!order && !sku) { previousOrder = ''; continue; }
    if (kind === 'physical') {
      if (order) previousOrder = order;
      else order = previousOrder;
    }
    if (!order && kind === 'payment') continue;
    if (!order) throw new Error(`${name} row ${index + 1}: Order ID is missing.`);
    if (!sku && kind !== 'payment' && !(marketplace === 'flipkart' && kind === 'physical')) throw new Error(`${name} row ${index + 1}: SKU is missing.`);
    let rowDate = '';
    let updated = '';
    if (kind === 'website') {
      const updatedValue = marketplace === 'flipkart'
        ? get('completed') || get('out_for_delivery') || get('approval') || get('date')
        : get('updated');
      try { updated = isoDate(updatedValue, marketplace === 'flipkart' ? 'Flipkart status date' : 'Last Updated Date'); }
      catch (error) { throw new Error(`${name} row ${index + 1}: ${(error as Error).message}`); }
      try { rowDate = clean(get('date')) ? isoDate(get('date'), 'Return Date') : updated; }
      catch (error) { throw new Error(`${name} row ${index + 1}: ${(error as Error).message}`); }
      if (!clean(get('status'))) throw new Error(`${name} row ${index + 1}: Return Status is missing.`);
    } else {
      try {
        if (kind === 'physical' && !clean(get('date'))) rowDate = isoDate(`${monthFromSheet(name, coverage)}-01`, 'Physical Receipt Date');
        else rowDate = isoDate(get('date'), kind === 'payment' ? 'Transaction Date' : 'Physical Receipt Date');
        updated = rowDate;
      } catch (error) { throw new Error(`${name} row ${index + 1}: ${(error as Error).message}`); }
    }
    let delivered = '';
    if (clean(get('delivered'))) delivered = isoDate(get('delivered'), 'Delivered Date');
    let amount = 0;
    const base: SourceRow = {
      import_id: importId,
      kind,
      order_id: order,
      sku,
      date: rowDate,
      updated,
      delivered,
      status: clean(get('status')),
      reason: clean(get('reason')),
      type: clean(get('type')),
      channel: clean(get('channel')),
      reference_id: normalize(get('reference')),
      amount,
      source: name,
      line: index + 1,
      imported_at: importedAt
    };
    if (kind === 'payment' && marketplace === 'flipkart' && (cols.sale !== undefined || cols.refund !== undefined || cols.protection !== undefined)) {
      const transactions: Array<[string, unknown]> = [
        ['SALE', get('sale')],
        ['REFUND', get('refund')],
        ['SPF REIMBURSEMENT', get('protection')]
      ];
      for (const [type, value] of transactions) {
        const transactionAmount = money(value);
        if (transactionAmount) output.push({ ...base, type, amount: transactionAmount });
      }
      const fee = money(get('marketplace_fee'));
      if (fee > 0) output.push({ ...base, type: 'FEE REFUND', amount: fee });
      continue;
    }
    if (kind === 'payment') {
      amount = money(get('amount'));
      if (!clean(get('type'))) throw new Error(`${name} row ${index + 1}: Payment Type is missing.`);
      base.amount = amount;
    }
    output.push(base);
  }
  if (!cols) {
    if (optionalSheet) return [];
    const expected = kind === 'website'
      ? 'Customer Order ID, mSKU, Last Updated On, and Return Status (Return Created/Pick-up Date is optional)'
      : kind === 'physical' ? 'Order Id and SKU' : 'date/time, type, order id, and total';
    throw new Error(`${name}: required ${kind} headers are missing. Expected ${expected}.`);
  }
  return output;
}

function workbookRows(bytes: Uint8Array, filename: string, marketplace: Marketplace, kind: SourceRow['kind'], coverage: string, importId: string, importedAt: string): SourceRow[] {
  const workbook = XLSX.read(bytes, { type: 'array', cellDates: false, raw: true });
  const rows: SourceRow[] = [];
  const optionalSheets = marketplace === 'flipkart' && workbook.SheetNames.length > 1;
  for (const sheetName of workbook.SheetNames) {
    const data = XLSX.utils.sheet_to_json<unknown[]>(workbook.Sheets[sheetName], { header: 1, raw: true, defval: '' });
    if (!data.some((row) => row.some((value) => clean(value)))) continue;
    rows.push(...parseSheet(data, filename.toLowerCase().endsWith('.csv') ? 'CSV' : sheetName, marketplace, kind, coverage, importId, importedAt, optionalSheets));
  }
  return rows;
}

export function parseBrowserUploadBytes(kind: string, filename: string, bytes: Uint8Array, coverage = '', importId = 'test-import', importedAt = new Date().toISOString(), marketplace: Marketplace = 'amazon'): SourceRow[] {
  if (!['website', 'physical', 'payment'].includes(kind)) throw new Error('Unknown import type.');
  const ext = filename.toLowerCase().match(/\.[^.]+$/)?.[0] ?? '';
  if (!['.csv', '.xlsx', '.zip'].includes(ext)) throw new Error('Unsupported file. Upload CSV, XLSX, or ZIP only.');
  let parsed: SourceRow[] = [];
  if (ext === '.zip') {
    let files: Record<string, Uint8Array>;
    try { files = unzipSync(bytes); }
    catch { throw new Error('The ZIP archive is invalid or damaged.'); }
    for (const [name, content] of Object.entries(files)) {
      if (name.endsWith('/') || name.startsWith('__MACOSX/') || name.endsWith('/.DS_Store')) continue;
      const entryExt = name.toLowerCase().match(/\.[^.]+$/)?.[0] ?? '';
      if (!['.csv', '.xlsx'].includes(entryExt)) throw new Error(`ZIP contains unsupported file ${JSON.stringify(name)}. Only CSV and XLSX are allowed.`);
      parsed.push(...workbookRows(content, name, marketplace, kind as SourceRow['kind'], coverage, importId, importedAt));
    }
  } else {
    parsed = workbookRows(bytes, filename, marketplace, kind as SourceRow['kind'], coverage, importId, importedAt);
  }
  if (!parsed.length) throw new Error('No data rows were found in the selected file.');
  return prepareRows(parsed);
}

async function hashBytes(bytes: Uint8Array): Promise<string> {
  if (globalThis.crypto?.subtle) {
    const data = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
    const digest = await globalThis.crypto.subtle.digest('SHA-256', data);
    return [...new Uint8Array(digest)].map((v) => v.toString(16).padStart(2, '0')).join('');
  }
  let hash = 2166136261;
  for (const value of bytes) hash = Math.imul(hash ^ value, 16777619);
  return `local-${(hash >>> 0).toString(16).padStart(8, '0')}-${bytes.length}`;
}

function rowIdentity(row: SourceRow) {
  if (row.kind === 'payment') {
    const base = `${row.order_id}\u001f${row.sku}\u001f${row.date.slice(0, 10)}\u001f${normalize(row.type)}`;
    return row.reference_id ? `${row.reference_id}\u001f${base}` : `${base}\u001f${row.amount}`;
  }
  if (row.kind === 'physical') return `${row.order_id}\u001f${row.sku}\u001f${row.date.slice(0, 10)}`;
  return `${row.order_id}\u001f${row.sku}\u001f${row.updated}\u001f${normalize(row.status)}`;
}

function prepareRows(rows: SourceRow[]) {
  const counts = new Map<string, number>();
  return rows.map((row) => {
    const identity = `${row.kind}\u001f${rowIdentity(row)}`;
    const occurrence = (counts.get(identity) ?? 0) + 1;
    counts.set(identity, occurrence);
    return { ...row, occurrence: row.kind === 'payment' ? occurrence : 1 };
  });
}

function activeRows(state: BrowserState) {
  const active = new Set(state.imports.filter((item) => item.active).map((item) => item.id));
  const selected = new Map<string, SourceRow>();
  const legacyCounts = new Map<string, number>();
  for (const row of state.rows) {
    if (!active.has(row.import_id)) continue;
    const identity = `${row.kind}\u001f${rowIdentity(row)}`;
    const legacyKey = `${row.import_id}\u001f${identity}`;
    const occurrence = row.occurrence ?? ((legacyCounts.get(legacyKey) ?? 0) + 1);
    legacyCounts.set(legacyKey, occurrence);
    const selectionKey = `${identity}\u001f${row.kind === 'payment' ? occurrence : 1}`;
    const previous = selected.get(selectionKey);
    if (!previous || previous.imported_at <= row.imported_at) selected.set(selectionKey, row);
  }
  return [...selected.values()];
}

export async function importBrowserFile(state: BrowserState, marketplace: Marketplace, kind: string, file: File, coverage: string): Promise<{ state: BrowserState; duplicate: boolean; imported: Import }> {
  if (!['website', 'physical', 'payment'].includes(kind)) throw new Error('Unknown import type.');
  const ext = file.name.toLowerCase().match(/\.[^.]+$/)?.[0] ?? '';
  if (!['.csv', '.xlsx', '.zip'].includes(ext)) throw new Error('Unsupported file. Upload CSV, XLSX, or ZIP only.');
  if (file.size > MAX_FILE_SIZE) throw new Error('File exceeds the 100 MB upload limit.');
  let bytes: Uint8Array;
  try { bytes = new Uint8Array(await file.arrayBuffer()); }
  catch (error) { throw new Error(`Could not read the selected file: ${(error as Error).message}`); }
  const sha256 = await hashBytes(bytes);
  const duplicate = state.imports.find((item) => item.sha256 === sha256 && item.kind === kind);
  if (duplicate) return { state, duplicate: true, imported: duplicate };
  const importedAt = new Date().toISOString();
  const importId = globalThis.crypto?.randomUUID?.() ?? `import-${Date.now()}-${Math.random().toString(16).slice(2)}`;
  const parsed = parseBrowserUploadBytes(kind, file.name, bytes, coverage, importId, importedAt, marketplace);
  const imported: Import = { id: importId, kind, filename: file.name, sha256, coverage_month: coverage, status: 'completed', active: true, rows: parsed.length, error: '', created_at: importedAt };
  const next: BrowserState = {
    ...state,
    // Keep rows scoped to their import so deletion can reveal an older active version.
    rows: [...state.rows, ...parsed],
    imports: [imported, ...state.imports],
    audit: [{ id: Date.now(), action: 'import.completed', details: { kind, filename: file.name, rows: parsed.length }, created_at: importedAt }, ...state.audit]
  };
  try { await saveBrowserState(next, marketplace); }
  catch (error) { throw new Error(`The file was parsed, but the browser could not save it: ${(error as Error).message}`); }
  return { state: next, duplicate: false, imported };
}

function addDays(value: string, days: number) {
  const date = new Date(value);
  date.setUTCDate(date.getUTCDate() + days);
  return date.toISOString();
}

function monthOf(value: string) {
  const date = new Date(value);
  const shifted = new Date(date.getTime() + 19800000);
  return `${shifted.getUTCFullYear()}-${String(shifted.getUTCMonth() + 1).padStart(2, '0')}`;
}

function nextMonth(value: string) {
  const [year, month] = value.split('-').map(Number);
  return month === 12 ? `${year + 1}-01` : `${year}-${String(month + 1).padStart(2, '0')}`;
}

const evidence = (row: SourceRow) => ({ kind: row.kind, order_id: row.order_id, sku: row.sku, date: row.date, type: row.type, amount: row.amount, source: row.source, line: row.line, status: row.status });

function reconcileAmazonBrowser(state: BrowserState): Case[] {
  const rows = activeRows(state);
  const website = new Map<string, SourceRow[]>(), physical = new Map<string, SourceRow[]>(), payments = new Map<string, SourceRow[]>(), orderSkus = new Map<string, Set<string>>(), blankPayments = new Map<string, SourceRow[]>();
  for (const row of rows) {
    const key = keyOf(row.order_id, row.sku);
    const map = row.kind === 'website' ? website : row.kind === 'physical' ? physical : payments;
    if (row.kind === 'payment' && !row.sku) blankPayments.set(row.order_id, [...(blankPayments.get(row.order_id) ?? []), row]);
    else map.set(key, [...(map.get(key) ?? []), row]);
    if (row.kind === 'website' && row.sku) {
      const skus = orderSkus.get(row.order_id) ?? new Set<string>();
      skus.add(row.sku); orderSkus.set(row.order_id, skus);
    }
  }
  const output: Case[] = [];
  const now = new Date();
  for (const [key, events] of website) {
    const ordered = [...events].sort((a, b) => a.updated.localeCompare(b.updated) || a.imported_at.localeCompare(b.imported_at));
    const latest = ordered[ordered.length - 1];
    const first = [...ordered].sort((a, b) => a.date.localeCompare(b.date))[0].date;
    const month = monthOf(first), following = nextMonth(month);
    const receivedRows = (physical.get(key) ?? []).filter((row) => [month, following].includes(monthOf(row.date)));
    const result: Case = {
      key, order_id: latest.order_id, sku: latest.sku, month, status: latest.status,
      receipt: receivedRows.length ? 'Received' : 'Not Received', received_month: receivedRows.length ? (receivedRows.some((row) => monthOf(row.date) === month) ? month : following) : '',
      financial: receivedRows.length ? 'Not Investigated — Received' : 'Payment Data Missing', scenario: '', order_total: 0, refund_total: 0, fee_refund: 0, reimbursement: 0, net: 0, loss: 0, recovered: 0,
      refund_date: '', updated: latest.updated, delivered: latest.delivered,
      claim_rule: '', claim_open: '', claim_deadline: '', claim_status: receivedRows.length ? 'No Claim' : 'Review',
      claim_raised: false, claim_raised_date: '', user_claim_status: 'Not Raised', claim_updated_date: '', amazon_claim_id: '', claim_amount_requested: 0, claim_amount_approved: 0, claim_rejection_reason: '', claim_notes: '', user_notes: '', rule_version: 'amazon-seller-v1', notes: [], evidence: [...ordered, ...receivedRows].map(evidence)
    };
    const manual = state.claims[key];
    if (manual) Object.assign(result, manual);
    if (receivedRows.length) { output.push(result); continue; }
    let tx = [...(payments.get(key) ?? [])];
    const orderLevel = blankPayments.get(latest.order_id) ?? [];
    if (orderLevel.length && orderSkus.get(latest.order_id)?.size === 1) { tx = [...tx, ...orderLevel]; result.notes.push('Unique order-level payment fallback'); }
    else if (orderLevel.length) result.notes.push('Unallocated blank-SKU payment: review multi-SKU order');
    if (!tx.length) { output.push(result); continue; }
    let hasRefund = false, unsupported = false;
    for (const payment of tx) {
      result.evidence!.push(evidence(payment));
      result.net += payment.amount;
      switch (normalize(payment.type)) {
        case 'ORDER': result.order_total += payment.amount; break;
        case 'REFUND': hasRefund = true; result.refund_total += payment.amount; if (!result.refund_date || payment.date < result.refund_date) result.refund_date = payment.date; break;
        case 'FULFILMENT FEE REFUND': case 'FULFILLMENT FEE REFUND': result.fee_refund += payment.amount; break;
        case 'SAFE-T REIMBURSEMENT': case 'SAFE T REIMBURSEMENT': case 'REIMBURSEMENTS': case 'REIMBURSEMENT': result.reimbursement += payment.amount; break;
        default: unsupported = true; result.notes.push(`Unclassified payment type: ${payment.type}`);
      }
    }
    result.scenario = hasRefund ? (result.order_total ? 'Order + Refund' : 'Refund Only') : 'Order Only';
    if (hasRefund && result.fee_refund) result.scenario += ' + Fulfilment Fee Refund';
    if (hasRefund && result.reimbursement) result.scenario += ' + Reimbursement';
    if (!hasRefund || result.refund_total >= 0) { result.financial = 'Not Debited'; result.claim_status = 'No Claim'; }
    else {
      result.financial = 'Debited';
      const gross = -result.refund_total;
      result.recovered = Math.max(0, Math.min(gross, result.reimbursement));
      result.loss = gross - result.recovered;
      if (!result.loss) { result.financial = 'Already Reimbursed'; result.claim_status = 'No Claim'; }
      else {
        if (result.recovered > 0) result.financial = 'Partially Reimbursed';
        const isFba = normalize(latest.channel).includes('FBA') || normalize(latest.channel).includes('AMAZON CENTRE');
        if (isFba) { result.claim_rule = 'FBA — separate workflow'; result.claim_status = 'Review'; }
        else {
          const delivered = ['RETURNED TO SELLER', 'DELIVERED TO SELLER'].includes(normalize(result.status));
          if (delivered) {
            result.claim_rule = 'Later delivered/refund + 15 days';
            result.delivered ||= latest.updated;
            result.claim_open = result.delivered > result.refund_date ? result.delivered : result.refund_date;
            result.claim_deadline = addDays(result.claim_open, 15);
          } else {
            result.claim_rule = 'Refund + 50–75 days'; result.claim_open = addDays(result.refund_date, 50); result.claim_deadline = addDays(result.refund_date, 75);
          }
          const today = now.toISOString().slice(0, 10), open = result.claim_open.slice(0, 10), deadline = result.claim_deadline.slice(0, 10);
          result.claim_status = today < open ? 'Scheduled' : today > deadline ? 'Expired' : 'Raise Claim';
        }
      }
    }
    if (unsupported || (orderLevel.length && (orderSkus.get(latest.order_id)?.size ?? 0) > 1)) { result.financial = 'Payment Review'; result.claim_status = 'Review'; result.notes.push('Financial allocation requires review before claiming'); }
    output.push(result);
  }
  return output.sort((a, b) => a.month.localeCompare(b.month) || a.key.localeCompare(b.key));
}

function claimWindow(result: Case, open: string, deadline: string, rule: string, now: Date) {
  result.claim_rule = rule;
  result.claim_open = open;
  result.claim_deadline = deadline;
  const today = now.toISOString().slice(0, 10);
  result.claim_status = today < open.slice(0, 10) ? 'Scheduled' : today > deadline.slice(0, 10) ? 'Expired' : 'Raise Claim';
}

function reconcileFlipkartBrowser(state: BrowserState): Case[] {
  const rows = activeRows(state);
  const website = new Map<string, SourceRow[]>(), physical = new Map<string, SourceRow[]>(), payments = new Map<string, SourceRow[]>();
  const orderKeys = new Map<string, Set<string>>(), itemKeys = new Map<string, Set<string>>();
  for (const row of rows.filter((item) => item.kind === 'website')) {
    const key = keyOf(row.order_id, row.sku);
    website.set(key, [...(website.get(key) ?? []), row]);
    const byOrder = orderKeys.get(row.order_id) ?? new Set<string>(); byOrder.add(key); orderKeys.set(row.order_id, byOrder);
    if (row.reference_id) { const byItem = itemKeys.get(row.reference_id) ?? new Set<string>(); byItem.add(key); itemKeys.set(row.reference_id, byItem); }
  }
  const resolveCase = (row: SourceRow) => {
    if (row.sku) { const direct = keyOf(row.order_id, row.sku); if (website.has(direct)) return direct; }
    const byItem = row.reference_id ? itemKeys.get(row.reference_id) : undefined;
    if (byItem?.size === 1) return [...byItem][0];
    const byOrder = orderKeys.get(row.order_id);
    if (byOrder?.size === 1) return [...byOrder][0];
    return '';
  };
  const ambiguous = new Map<string, number>();
  for (const row of rows.filter((item) => item.kind !== 'website')) {
    const key = resolveCase(row);
    if (!key) { ambiguous.set(row.order_id, (ambiguous.get(row.order_id) ?? 0) + 1); continue; }
    const target = row.kind === 'physical' ? physical : payments;
    target.set(key, [...(target.get(key) ?? []), row]);
  }
  const output: Case[] = [], now = new Date();
  for (const [key, events] of website) {
    const ordered = [...events].sort((a, b) => a.updated.localeCompare(b.updated) || a.imported_at.localeCompare(b.imported_at));
    const latest = ordered[ordered.length - 1];
    const first = [...ordered].sort((a, b) => a.date.localeCompare(b.date))[0].date;
    const receivedRows = [...(physical.get(key) ?? [])].sort((a, b) => a.date.localeCompare(b.date));
    const received = receivedRows.length > 0;
    const result: Case = {
      key, order_id: latest.order_id, sku: latest.sku, month: monthOf(first), status: latest.status,
      receipt: received ? 'Received' : 'Not Received', received_month: received ? monthOf(receivedRows[0].date) : '',
      financial: 'Payment Data Missing', scenario: '', order_total: 0, refund_total: 0, fee_refund: 0,
      reimbursement: 0, net: 0, loss: 0, recovered: 0, refund_date: '', updated: latest.updated,
      delivered: received ? receivedRows[0].date : latest.delivered, claim_rule: '', claim_open: '', claim_deadline: '',
      claim_status: 'Review', claim_raised: false, claim_raised_date: '', user_claim_status: 'Not Raised',
      claim_updated_date: '', amazon_claim_id: '', claim_amount_requested: 0, claim_amount_approved: 0,
      claim_rejection_reason: '', claim_notes: '', user_notes: '', rule_version: 'flipkart-spf-v1', notes: [],
      evidence: [...ordered, ...receivedRows].map(evidence)
    };
    if (ambiguous.get(latest.order_id)) result.notes.push(`${ambiguous.get(latest.order_id)} Flipkart row(s) need Order Item ID or SKU for allocation.`);
    const manual = state.claims[key];
    if (manual) Object.assign(result, manual);
    const tx = payments.get(key) ?? [];
    let hasRefund = false, unsupported = false;
    for (const payment of tx) {
      result.evidence!.push(evidence(payment));
      result.net += payment.amount;
      const type = normalize(payment.type);
      if (['ORDER', 'SALE', 'SALE OF PRODUCT', 'PRODUCT SALE'].includes(type)) result.order_total += payment.amount;
      else if (['REFUND', 'RETURN', 'CUSTOMER REFUND', 'REFUND FOR RETURN', 'RETURN REFUND'].includes(type)) {
        hasRefund = true; result.refund_total -= Math.abs(payment.amount);
        if (!result.refund_date || payment.date < result.refund_date) result.refund_date = payment.date;
      } else if (type.includes('SPF') || type.includes('SELLER PROTECTION FUND') || type.includes('REIMBURSEMENT')) {
        result.reimbursement += Math.abs(payment.amount);
      } else if (type.includes('FEE REFUND') || type.includes('COMMISSION REFUND')) result.fee_refund += Math.abs(payment.amount);
      else { unsupported = true; result.notes.push(`Unclassified Flipkart payment type: ${payment.type}`); }
    }
    result.scenario = hasRefund ? (result.order_total ? 'Sale + Refund' : 'Refund Only') : 'Sale Only';
    if (result.reimbursement) result.scenario += ' + SPF Recovery';
    if (!tx.length) result.financial = received ? 'Not Investigated — Received' : 'Payment Data Missing';
    else if (!hasRefund) result.financial = 'Not Debited';
    else {
      const gross = Math.abs(result.refund_total);
      result.recovered = Math.min(gross, Math.max(0, result.reimbursement));
      result.loss = gross - result.recovered;
      result.financial = result.loss === 0 ? 'Already Reimbursed' : result.recovered > 0 ? 'Partially Reimbursed' : 'Debited';
    }
    const classification = normalize(`${latest.status} ${latest.reason} ${latest.channel}`);
    if (received && ['DAMAGED', 'DEFECTIVE', 'WRONG ITEM'].some((value) => classification.includes(value))) {
      claimWindow(result, receivedRows[0].date, addDays(receivedRows[0].date, 14), 'SPF received condition claim · within 14 days', now);
    } else if (received && classification.includes('IN TRANSIT')) {
      claimWindow(result, addDays(receivedRows[0].date, 1), addDays(first, 120), 'In transit but physically received · wait 24 hours', now);
    } else if (!received && (hasRefund || classification.includes('DO NOT EXPECT') || classification.includes('DNE'))) {
      claimWindow(result, addDays(first, 60), addDays(first, 120), classification.includes('DO NOT EXPECT') || classification.includes('DNE') ? 'DNE physical delivery · day 60 to day 120' : 'Product Missing · day 60 to day 120', now);
    } else if (result.financial === 'Already Reimbursed' || result.financial === 'Not Debited' || received) result.claim_status = 'No Claim';
    else result.claim_status = 'Review';
    if (unsupported) { result.financial = 'Payment Review'; result.claim_status = 'Review'; result.notes.push('Flipkart payment allocation requires review.'); }
    output.push(result);
  }
  return output.sort((a, b) => a.month.localeCompare(b.month) || a.key.localeCompare(b.key));
}

export function reconcileBrowser(state: BrowserState, marketplace: Marketplace = 'amazon'): Case[] {
  return marketplace === 'flipkart' ? reconcileFlipkartBrowser(state) : reconcileAmazonBrowser(state);
}

export async function saveBrowserClaim(state: BrowserState, item: Case, marketplace: Marketplace = 'amazon'): Promise<BrowserState> {
  const previous = state.claims[item.key] ?? null;
  const claim: ClaimData = {
    claim_raised: item.claim_raised, claim_raised_date: item.claim_raised_date, user_claim_status: item.user_claim_status,
    claim_updated_date: item.claim_updated_date, amazon_claim_id: item.amazon_claim_id,
    claim_amount_requested: item.claim_amount_requested, claim_amount_approved: item.claim_amount_approved,
    claim_rejection_reason: item.claim_rejection_reason, claim_notes: item.claim_notes, user_notes: item.user_notes
  };
  const next = { ...state, claims: { ...state.claims, [item.key]: claim }, audit: [{ id: Date.now(), action: 'claim.updated', details: { case_key: item.key, previous, current: claim }, created_at: item.claim_updated_date }, ...state.audit] };
  await saveBrowserState(next, marketplace); return next;
}

export async function setBrowserImportActive(state: BrowserState, id: string, active: boolean, marketplace: Marketplace = 'amazon'): Promise<BrowserState> {
  const next = { ...state, imports: state.imports.map((item) => item.id === id ? { ...item, active } : item), audit: [{ id: Date.now(), action: active ? 'import.restored' : 'import.excluded', details: { import_id: id }, created_at: new Date().toISOString() }, ...state.audit] };
  await saveBrowserState(next, marketplace); return next;
}

export function withoutBrowserImport(state: BrowserState, id: string): BrowserState {
  const imported = state.imports.find((item) => item.id === id);
  if (!imported) throw new Error('Import not found.');
  const createdAt = new Date().toISOString();
  return {
    ...state,
    rows: state.rows.filter((row) => row.import_id !== id),
    imports: state.imports.filter((item) => item.id !== id),
    // Claim entries are intentionally retained so user decisions survive file correction.
    audit: [{ id: Date.now(), action: 'import.deleted', details: { import_id: id, filename: imported.filename, kind: imported.kind, rows: imported.rows }, created_at: createdAt }, ...state.audit]
  };
}

export async function deleteBrowserImport(state: BrowserState, id: string, marketplace: Marketplace = 'amazon'): Promise<BrowserState> {
  const next = withoutBrowserImport(state, id);
  await saveBrowserState(next, marketplace);
  return next;
}
