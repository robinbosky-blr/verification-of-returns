export type Summary = {
  total: number;
  received: number;
  not_received: number;
  debited: number;
  not_debited: number;
  unknown: number;
  reimbursed: number;
  partial: number;
  raise: number;
  scheduled: number;
  expired: number;
  refund_debited: number;
  reimbursement: number;
  fee_refund: number;
  loss: number;
  recovered: number;
  no_loss_cases: number;
  claim_pending: number;
  claim_approved: number;
  claim_rejected: number;
  claim_partial_approved: number;
};
export type Case = {
  key: string;
  order_id: string;
  sku: string;
  month: string;
  status: string;
  receipt: string;
  received_month: string;
  financial: string;
  scenario: string;
  order_total: number;
  refund_total: number;
  fee_refund: number;
  reimbursement: number;
  net: number;
  loss: number;
  recovered: number;
  refund_date: string;
  updated: string;
  delivered: string;
  claim_rule: string;
  claim_open: string;
  claim_deadline: string;
  claim_status: string; // automated System Claim Action
  claim_raised: boolean;
  claim_raised_date: string;
  user_claim_status: string;
  claim_updated_date: string;
  amazon_claim_id: string;
  claim_amount_requested: number;
  claim_amount_approved: number;
  claim_rejection_reason: string;
  claim_notes: string;
  user_notes: string;
  rule_version: string;
  notes: string[];
  evidence?: {
    kind: string;
    order_id: string;
    sku: string;
    date: string;
    type: string;
    amount: number;
    source: string;
    line: number;
    status: string;
  }[];
};
export type Import = {
  id: string;
  kind: string;
  filename: string;
  sha256: string;
  coverage_month: string;
  status: string;
  active: boolean;
  rows: number;
  error: string;
  created_at: string;
};
export type Results = {
  rows: Case[];
  total: number;
  summary: Summary;
  monthly: Record<string, Summary>;
  page: number;
  limit: number;
};
export type Meta = {
  months: { month: string; count: number }[];
  jobs: { id: number; kind: string; status: string; error: string; created_at: string }[];
  rule_version: string;
};
export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const r = await fetch('/api' + path, options);
  const d = await r.json();
  if (!r.ok) throw new Error(d.error || 'Request failed');
  return d as T;
}
export const money = (v: number = 0) =>
  new Intl.NumberFormat('en-IN', {
    style: 'currency',
    currency: 'INR',
    maximumFractionDigits: 2
  }).format(v / 100);
export const number = (v: number = 0) => new Intl.NumberFormat('en-IN').format(v);
export const date = (s: string) =>
  !s || s.startsWith('0001')
    ? '—'
    : new Intl.DateTimeFormat('en-IN', {
        day: '2-digit',
        month: 'short',
        year: 'numeric',
        timeZone: 'Asia/Kolkata'
      }).format(new Date(s));
export const monthLabel = (s: string) =>
  new Intl.DateTimeFormat('en-IN', {
    month: 'short',
    year: 'numeric',
    timeZone: 'Asia/Kolkata'
  }).format(new Date(s + '-01T00:00:00+05:30'));
