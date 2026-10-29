<script lang="ts">
  import { onMount } from 'svelte';
  import * as XLSX from 'xlsx';
  import {
    LayoutDashboard,
    CloudUpload,
    Table2,
    ShieldCheck,
    Moon,
    Sun,
    ArrowUpRight,
    ArrowRight,
    ArrowDownToLine,
    Search,
    ChevronDown,
    ChevronLeft,
    ChevronRight,
    Plus,
    Check,
    Package,
    PackageCheck,
    PackageX,
    IndianRupee,
    RefreshCw,
    X,
    LogOut,
    AlertCircle,
    Layers,
    FileSpreadsheet,
    CheckCircle2,
    LoaderCircle,
    Trash2
  } from '@lucide/svelte';
  import {
    api,
    money,
    number,
    date,
    monthLabel,
    type Summary,
    type Case,
    type Results,
    type Import,
    type Meta
  } from '../lib/api';
  import {
    importBrowserFile,
    deleteBrowserImport,
    loadBrowserState,
    reconcileBrowser,
    saveBrowserClaim,
    setBrowserImportActive,
    type BrowserState,
    type Marketplace
  } from '../lib/browser-store';
  import './style.css';
  type Page = 'Overview' | 'Data imports' | 'Reports' | 'Claims';
  let page = $state<Page>('Overview'),
    signed = $state(false),
    checking = $state(true),
    browserMode = $state(false),
    marketplace = $state<Marketplace>('amazon'),
    theme = $state<'light' | 'dark'>('light'),
    password = $state(''),
    loginError = $state('');
  let busy = $state(false),
    error = $state(''),
    notice = $state(''),
    month = $state(''),
    year = $state(''),
    search = $state(''),
    tab = $state('All returns'),
    claim = $state(''),
    financial = $state(''),
    status = $state(''),
    pageNo = $state(1);
  let results = $state<Results | null>(null),
    overview = $state<Results | null>(null),
    imports = $state<Import[]>([]),
    meta = $state<Meta>({ months: [], jobs: [], rule_version: 'amazon-seller-v1' });
  let selected = $state<Case | null>(null),
    uploading = $state(''),
    deleting = $state(''),
    coverage = $state(''),
    menu = $state(false);
  let browserState = $state<BrowserState>({ version: 1, rows: [], imports: [], claims: {}, audit: [] });
  let dashboardPreview = $state<{ title: string; rows: Case[]; total: number } | null>(null);
  let claimForm = $state({
    status: 'Not Raised', raisedDate: '', amazonClaimID: '', requested: '', approved: '', rejectionReason: '', notes: '', userNotes: ''
  });
  const nav = [
    { name: 'Overview' as Page, icon: LayoutDashboard },
    { name: 'Data imports' as Page, icon: CloudUpload },
    { name: 'Reports' as Page, icon: Table2 },
    { name: 'Claims' as Page, icon: ShieldCheck }
  ];
  const tabs = [
    'Summary',
    'All returns',
    'Received',
    'Not Received',
    'Debited',
    'Not Debited',
    'Reimbursed'
  ];
  const amazonCards = [
    { kind: 'website', title: 'Website return data', desc: 'Amazon return reconciliation reports', icon: Package, color: 'blue' },
    { kind: 'physical', title: 'Physical received data', desc: 'Your warehouse receipt records', icon: PackageCheck, color: 'blue' },
    { kind: 'payment', title: 'Payment data', desc: 'Amazon transaction and settlement files', icon: IndianRupee, color: 'blue' }
  ];
  const flipkartCards = [
    { kind: 'website', title: 'Flipkart return data', desc: 'Flipkart returns and status report', icon: Package, color: 'blue' },
    { kind: 'physical', title: 'Physical received data', desc: 'Flipkart warehouse receipt records', icon: PackageCheck, color: 'blue' },
    { kind: 'payment', title: 'Payment and SPF data', desc: 'Flipkart settlements, refunds and SPF recovery', icon: IndianRupee, color: 'blue' }
  ];
  let cards = $derived(marketplace === 'amazon' ? amazonCards : flipkartCards);
  function summarize(rows: Case[]): Summary {
    const s: Summary = { total: 0, received: 0, not_received: 0, debited: 0, not_debited: 0, unknown: 0, reimbursed: 0, partial: 0, raise: 0, scheduled: 0, expired: 0, refund_debited: 0, reimbursement: 0, fee_refund: 0, loss: 0, recovered: 0, no_loss_cases: 0, claim_pending: 0, claim_approved: 0, claim_rejected: 0, claim_partial_approved: 0 };
    for (const c of rows) {
      s.total++;
      const manual = c.user_claim_status || 'Not Raised';
      if (manual === 'Not Raised') {
        if (c.claim_status === 'Raise Claim') s.raise++;
        if (c.claim_status === 'Scheduled') s.scheduled++;
        if (c.claim_status === 'Expired') s.expired++;
      }
      if (manual === 'Pending') s.claim_pending++;
      if (manual === 'Approved') s.claim_approved++;
      if (manual === 'Rejected') s.claim_rejected++;
      if (manual === 'Partially Approved') s.claim_partial_approved++;
      if (c.receipt === 'Received') { s.received++; continue; }
      s.not_received++;
      if (['Debited', 'Partially Reimbursed', 'Already Reimbursed'].includes(c.financial)) s.debited++;
      else if (c.financial === 'Not Debited') s.not_debited++;
      else s.unknown++;
      if (c.financial === 'Already Reimbursed') s.reimbursed++;
      if (c.financial === 'Partially Reimbursed') s.partial++;
      if (c.financial === 'Not Debited' || c.financial === 'Already Reimbursed') s.no_loss_cases++;
      if (c.refund_total < 0) s.refund_debited -= c.refund_total;
      s.reimbursement += c.reimbursement;
      s.fee_refund += c.fee_refund;
      s.loss += c.loss;
      s.recovered += c.recovered;
    }
    return s;
  }
  function refreshBrowser() {
    const allCases = reconcileBrowser(browserState, marketplace);
    const periodRows = allCases.filter((c) => (!month || c.month === month) && (!year || c.month.startsWith(year)));
    const monthly: Record<string, Summary> = {};
    for (const m of [...new Set(allCases.map((c) => c.month))]) monthly[m] = summarize(allCases.filter((c) => c.month === m));
    overview = { rows: periodRows, total: periodRows.length, summary: summarize(periodRows), monthly, page: 1, limit: 25 };
    let rows = [...periodRows];
    if (search) rows = rows.filter((c) => `${c.order_id} ${c.sku}`.toLowerCase().includes(search.toLowerCase()));
    if (tab === 'Received' || tab === 'Not Received') rows = rows.filter((c) => c.receipt === tab);
    if (tab === 'Debited') rows = rows.filter((c) => ['Debited', 'Partially Reimbursed', 'Already Reimbursed'].includes(c.financial));
    if (tab === 'Not Debited') rows = rows.filter((c) => c.financial === 'Not Debited');
    if (tab === 'Reimbursed') rows = rows.filter((c) => c.financial === 'Already Reimbursed');
    if (financial) rows = rows.filter((c) => c.financial === financial);
    if (status) rows = rows.filter((c) => c.status === status);
    if (page === 'Claims') {
      if (marketplace === 'amazon') rows = rows.filter((c) => c.receipt === 'Not Received');
      const q = claim || 'Raise Claim';
      rows = ['Pending', 'Approved', 'Rejected', 'Partially Approved'].includes(q)
        ? rows.filter((c) => c.user_claim_status === q)
        : rows.filter((c) => (c.user_claim_status || 'Not Raised') === 'Not Raised' && c.claim_status === q);
    }
    results = { rows, total: rows.length, summary: summarize(rows), monthly, page: 1, limit: 25 };
    imports = browserState.imports;
    meta = { months: Object.keys(monthly).sort().map((m) => ({ month: m, count: monthly[m].total })), jobs: [], rule_version: marketplace === 'amazon' ? 'amazon-seller-v1' : 'flipkart-spf-v1' };
  }
  let summary = $derived(overview?.summary);
  let months = $derived(Object.keys(overview?.monthly ?? {}).sort());
  const currentYear = new Intl.DateTimeFormat('en', { year: 'numeric', timeZone: 'Asia/Kolkata' }).format(new Date());
  let years = $derived([...new Set([currentYear, ...meta.months.map((x) => x.month.slice(0, 4))])].sort().reverse());
  let filterMonths = $derived(Array.from({ length: 12 }, (_, index) => `${year || currentYear}-${String(index + 1).padStart(2, '0')}`));
  function params(report = true) {
    const p = new URLSearchParams({ page: String(pageNo), limit: '25' });
    p.set('marketplace', marketplace);
    if (month) p.set('month', month);
    if (year) p.set('year', year);
    if (search && report) p.set('search', search);
    if (report) {
      if (tab === 'Received' || tab === 'Not Received') p.set('receipt', tab);
      if (tab === 'Debited' || tab === 'Not Debited') p.set('financial', tab);
      if (tab === 'Reimbursed') p.set('financial', 'Already Reimbursed');
      if (financial) p.set('financial', financial);
      if (status) p.set('status', status);
      if (page === 'Claims') {
        if (marketplace === 'amazon') p.set('receipt', 'Not Received');
        const userStatuses = ['Pending', 'Approved', 'Rejected', 'Partially Approved'];
        if (userStatuses.includes(claim)) p.set('claim_status', claim);
        else p.set('claim', claim || 'Raise Claim');
      }
    }
    return p.toString();
  }
  async function switchMarketplace(next: Marketplace) {
    if (next === marketplace) return;
    marketplace = next;
    localStorage.setItem('returnscope-marketplace', next);
    month = ''; year = ''; search = ''; financial = ''; status = ''; claim = ''; tab = 'All returns'; pageNo = 1;
    selected = null; dashboardPreview = null; notice = ''; error = '';
    if (browserMode) {
      busy = true;
      try { browserState = await loadBrowserState(next); refreshBrowser(); }
      catch (e) { error = (e as Error).message; }
      finally { busy = false; }
      return;
    }
    await load();
  }
  let loadSequence = 0;
  async function load() {
    if (!signed) return;
    if (browserMode) {
      refreshBrowser();
      busy = false;
      error = '';
      return;
    }
    const sequence = ++loadSequence;
    busy = true;
    error = '';
    try {
      const [r, o, i, m] = await Promise.all([
        api<Results>('/results?' + params()),
        api<Results>('/results?' + params(false)),
        api<Import[]>('/imports?marketplace=' + marketplace),
        api<Meta>('/meta?marketplace=' + marketplace)
      ]);
      if (sequence !== loadSequence) return;
      results = r;
      overview = o;
      imports = i;
      meta = m;
    } catch (e) {
      error = (e as Error).message;
    } finally {
      if (sequence === loadSequence) busy = false;
    }
  }
  async function login() {
    loginError = '';
    try {
      await api('/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password })
      });
      signed = true;
      password = '';
      await load();
    } catch (e) {
      loginError = (e as Error).message;
    }
  }
  async function logout() {
    if (browserMode) {
      notice = 'Your private browser workspace remains signed in on this device.';
      return;
    }
    await api('/logout', { method: 'POST' });
    signed = false;
    results = null;
    overview = null;
    selected = null;
  }
  function navigate(p: Page) {
    page = p;
    tab = 'All returns';
    claim = '';
    search = '';
    financial = '';
    status = '';
    pageNo = 1;
    menu = false;
    void load();
  }
  async function upload(kind: string, event: Event) {
    const input = event.target as HTMLInputElement;
    const files = [...(input.files ?? [])];
    if (!files.length) return;
    uploading = kind;
    error = '';
    notice = '';
    try {
      if (browserMode) {
        let nextState = browserState;
        let importedFiles = 0, duplicateFiles = 0, processedRows = 0;
        for (const file of files) {
          const imported = await importBrowserFile(nextState, marketplace, kind, file, coverage);
          nextState = imported.state;
          if (imported.duplicate) duplicateFiles++;
          else { importedFiles++; processedRows += imported.imported.rows; }
        }
        browserState = nextState;
        notice = `${number(importedFiles)} file${importedFiles === 1 ? '' : 's'} imported. ${number(processedRows)} rows were processed and the dashboard was recalculated.${duplicateFiles ? ` ${number(duplicateFiles)} duplicate file${duplicateFiles === 1 ? '' : 's'} skipped.` : ''}`;
        refreshBrowser();
        return;
      }
      let queued = 0, duplicates = 0;
      for (const file of files) {
        const data = new FormData();
        data.append('kind', kind);
        data.append('marketplace', marketplace);
        data.append('file', file);
        if (coverage) data.append('month', coverage);
        const r = await api<{ duplicate: boolean; import: Import }>('/imports', { method: 'POST', body: data });
        if (r.duplicate) duplicates++; else queued++;
      }
      notice = `${number(queued)} file${queued === 1 ? '' : 's'} preserved and queued for processing.${duplicates ? ` ${number(duplicates)} duplicate file${duplicates === 1 ? '' : 's'} skipped.` : ''}`;
      await load();
    } catch (e) {
      error = (e as Error).message;
    } finally {
      uploading = '';
      input.value = '';
    }
  }
  async function activate(i: Import) {
    if (browserMode) {
      browserState = await setBrowserImportActive(browserState, i.id, !i.active, marketplace);
      notice = i.active ? 'Import excluded. Affected cases were recalculated.' : 'Import restored. Affected cases were recalculated.';
      refreshBrowser();
      return;
    }
    try {
      await api('/imports/' + i.id + '/active?marketplace=' + marketplace, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ active: !i.active })
      });
      notice = 'Import updated. Affected cases are being recalculated.';
      await load();
    } catch (e) {
      error = (e as Error).message;
    }
  }
  async function removeImport(i: Import) {
    if (!globalThis.confirm(`Delete ${i.filename}?\n\nThe file's imported data will be removed and all totals will be recalculated. Manual claim IDs, statuses, and notes will be kept.`)) return;
    deleting = i.id;
    error = '';
    notice = '';
    try {
      if (browserMode) {
        browserState = await deleteBrowserImport(browserState, i.id, marketplace);
        notice = `${i.filename} deleted. Its imported data was removed and totals were recalculated.`;
        refreshBrowser();
        return;
      }
      await api('/imports/' + i.id + '?marketplace=' + marketplace, { method: 'DELETE' });
      notice = `${i.filename} deleted. Affected totals are being recalculated.`;
      await load();
    } catch (e) {
      error = (e as Error).message;
    } finally {
      deleting = '';
    }
  }
  async function detail(c: Case) {
    try {
      const detailCase = browserMode ? c : await api<Case>('/case?marketplace=' + marketplace + '&key=' + encodeURIComponent(c.key));
      selected = detailCase;
      claimForm = {
        status: detailCase.user_claim_status || 'Not Raised',
        raisedDate: detailCase.claim_raised_date && !detailCase.claim_raised_date.startsWith('0001') ? detailCase.claim_raised_date.slice(0, 10) : '',
        amazonClaimID: detailCase.amazon_claim_id || '',
        requested: detailCase.claim_amount_requested ? String(detailCase.claim_amount_requested / 100) : '',
        approved: detailCase.claim_amount_approved ? String(detailCase.claim_amount_approved / 100) : '',
        rejectionReason: detailCase.claim_rejection_reason || '',
        notes: detailCase.claim_notes || '',
        userNotes: detailCase.user_notes || ''
      };
    } catch (e) {
      error = (e as Error).message;
    }
  }
  async function openDashboardPreview(kind: string) {
    const p = new URLSearchParams({ page: '1', limit: '100' });
    if (month) p.set('month', month);
    if (year) p.set('year', year);
    if (kind === 'Received') p.set('receipt', 'Received');
    if (kind === 'Not Received') p.set('receipt', 'Not Received');
    if (kind === 'Debited') { p.set('receipt', 'Not Received'); p.set('financial', 'Debited'); }
    if (kind === 'Not Debited') { p.set('receipt', 'Not Received'); p.set('financial', 'Not Debited'); }
    try {
      if (browserMode) {
        let rows = [...(overview?.rows ?? [])];
        if (kind === 'Received' || kind === 'Not Received') rows = rows.filter((c) => c.receipt === kind);
        if (kind === 'Debited') rows = rows.filter((c) => c.receipt === 'Not Received' && ['Debited', 'Partially Reimbursed', 'Already Reimbursed'].includes(c.financial));
        if (kind === 'Not Debited') rows = rows.filter((c) => c.receipt === 'Not Received' && c.financial === 'Not Debited');
        dashboardPreview = { title: kind === 'All returns' ? 'All returns' : kind, rows, total: rows.length };
        return;
      }
      const r = await api<Results>('/results?' + p.toString());
      dashboardPreview = { title: kind === 'All returns' ? 'All returns' : kind, rows: r.rows, total: r.total };
    } catch (e) { error = (e as Error).message; }
  }
  async function saveClaim() {
    if (!selected) return;
    const raised = claimForm.status !== 'Not Raised';
    const body = {
      case_key: selected.key,
      claim_raised: raised,
      claim_raised_date: raised && claimForm.raisedDate ? new Date(claimForm.raisedDate + 'T00:00:00+05:30').toISOString() : null,
      claim_status: claimForm.status,
      amazon_claim_id: claimForm.amazonClaimID.trim(),
      claim_amount_requested: Math.round((Number(claimForm.requested) || 0) * 100),
      claim_amount_approved: Math.round((Number(claimForm.approved) || 0) * 100),
      claim_rejection_reason: claimForm.rejectionReason.trim(),
      claim_notes: claimForm.notes.trim(),
      user_notes: claimForm.userNotes.trim()
    };
    try {
      if (browserMode) {
        const updated: Case = {
          ...selected,
          claim_raised: raised,
          claim_raised_date: body.claim_raised_date ?? '',
          user_claim_status: body.claim_status,
          claim_updated_date: new Date().toISOString(),
          amazon_claim_id: body.amazon_claim_id,
          claim_amount_requested: body.claim_amount_requested,
          claim_amount_approved: body.claim_amount_approved,
          claim_rejection_reason: body.claim_rejection_reason,
          claim_notes: body.claim_notes,
          user_notes: body.user_notes
        };
        browserState = await saveBrowserClaim(browserState, updated, marketplace);
        selected = updated;
        refreshBrowser();
        notice = 'Claim status saved in this private browser workspace. New imports will not overwrite it.';
        return;
      }
      await api('/case/claim?marketplace=' + marketplace, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
      notice = `Claim status saved permanently. New ${marketplace === 'amazon' ? 'Amazon' : 'Flipkart'} imports will not overwrite this manual action.`;
      await detail(selected);
      await load();
    } catch (e) { error = (e as Error).message; }
  }
  async function reconcile() {
    try {
      if (browserMode) {
        notice = 'Reconciliation completed from your active imports.';
        refreshBrowser();
        return;
      }
      await api('/reconcile?marketplace=' + marketplace, { method: 'POST' });
      notice = 'Reconciliation queued. Results will update after it completes.';
      await load();
    } catch (e) {
      error = (e as Error).message;
    }
  }
  function exportFile(format: string) {
    if (browserMode) {
      notice = `Your ${format.toUpperCase()} export is being prepared from this browser workspace.`;
      exportBrowser(format);
      return;
    }
    window.location.href = '/api/export?' + params() + '&format=' + format;
  }
  function exportBrowser(format: string) {
    const rows = (results?.rows ?? []).map((c) => ({
      'Order ID': c.order_id,
      SKU: c.sku,
      Month: c.month,
      'Return Status': c.status,
      'Physical Status': c.receipt,
      'Received Month': c.received_month,
      'Financial Status': c.financial,
      Refund: c.refund_total / 100,
      [marketplace === 'amazon' ? 'Fulfilment Fee Refund' : 'Flipkart Fee Refund']: c.fee_refund / 100,
      [marketplace === 'amazon' ? 'Reimbursement' : 'SPF Reimbursement']: c.reimbursement / 100,
      'Current Product Loss': c.loss / 100,
      'System Claim Action': c.claim_status,
      'User Claim Status': c.user_claim_status,
      'Claim Updated Date': c.claim_updated_date,
      [marketplace === 'amazon' ? 'Amazon Claim ID' : 'Flipkart SPF Claim ID']: c.amazon_claim_id,
      'Requested Amount': c.claim_amount_requested / 100,
      'Approved Amount': c.claim_amount_approved / 100,
      'Rejection Reason': c.claim_rejection_reason,
      'Claim Notes': c.claim_notes,
      'User Notes': c.user_notes
    }));
    const sheet = XLSX.utils.json_to_sheet(rows);
    const stamp = new Date().toISOString().slice(0, 10);
    if (format === 'csv') {
      const blob = new Blob([XLSX.utils.sheet_to_csv(sheet)], { type: 'text/csv;charset=utf-8' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url; link.download = `returnscope-${marketplace}-${stamp}.csv`; link.click(); URL.revokeObjectURL(url);
      return;
    }
    const book = XLSX.utils.book_new();
    XLSX.utils.book_append_sheet(book, sheet, 'Return cases');
    XLSX.writeFile(book, `returnscope-${marketplace}-${stamp}.xlsx`);
  }
  function tone(s: string) {
    if (['Received', 'Already Reimbursed', 'Not Debited', 'completed', 'No Claim', 'Approved'].includes(s))
      return 'success';
    if (['Not Received', 'Expired', 'failed', 'Debited', 'Rejected'].includes(s)) return 'danger';
    if (['Raise Claim', 'Partially Reimbursed', 'Scheduled', 'queued', 'Pending', 'Partially Approved'].includes(s))
      return 'warning';
    return 'neutral';
  }
  function changeTheme(next: 'light' | 'dark') {
    theme = next;
    localStorage.setItem('returnscope-theme', next);
  }
  onMount(() => {
    let alive = true;
    theme = localStorage.getItem('returnscope-theme') === 'dark' ? 'dark' : 'light';
    marketplace = localStorage.getItem('returnscope-marketplace') === 'flipkart' ? 'flipkart' : 'amazon';
    if (location.hostname === 'terminal.local' || location.hostname.endsWith('.chatgpt.site')) {
      browserMode = true;
      signed = true;
      checking = false;
      void loadBrowserState(marketplace)
        .then((saved) => { browserState = saved; refreshBrowser(); })
        .catch((e) => { error = (e as Error).message; });
      return;
    }
    void api('/session')
      .then(async () => {
        if (alive) {
          signed = true;
          await load();
        }
      })
      .catch(() => {})
      .finally(() => (checking = false));
    const timer = setInterval(() => {
      if (signed && !busy && !uploading) void load();
    }, 5000);
    return () => {
      alive = false;
      clearInterval(timer);
    };
  });
</script>

<svelte:head
  ><title>ReturnScope · Returns reconciliation</title><meta
    name="description"
    content="Marketplace returns, payments and claims in one clear workspace."
  /></svelte:head
>
{#if checking}<div class="loading-screen">
    <LoaderCircle class="spin" size={28} />
    <p>Opening your workspace…</p>
  </div>
{:else if !signed}
  <main class="login">
    <div class="login-story">
      <div class="brand">
        <div class="brandmark"><Layers size={23} /></div>
        ReturnScope<span>by Robinbosky</span>
      </div>
      <div>
        <div class="eyebrow">YOUR RETURNS, RECONCILED.</div>
        <h1>A clear picture.<br />Every return.</h1>
        <p>
          Connect your returns, warehouse receipts and payments. See what came back, what is still
          missing, and what needs a claim.
        </p>
        <div class="login-proof">
          <ShieldCheck size={22} />Marketplace-specific rules. Every calculation accounted for.
        </div>
      </div>
      <small>Returns intelligence for your business</small>
    </div>
    <form
      class="login-form"
      onsubmit={(e) => {
        e.preventDefault();
        void login();
      }}
    >
      <span class="pill success">SECURE WORKSPACE</span>
      <h2>Welcome back</h2>
      <p>Sign in to your returns workspace.</p>
      <label for="password">Workspace password</label><input
        id="password"
        type="password"
        bind:value={password}
        autocomplete="current-password"
        required
        placeholder="Enter your password"
      />{#if loginError}<div class="alert error">{loginError}</div>{/if}<button
        class="primary"
        type="submit">Sign in <ArrowRight size={17} /></button
      ><small>Your files and financial records stay in your workspace.</small>
    </form>
  </main>
{:else}
  <div class="app-shell" class:dark={theme === 'dark'}>
    <aside class:mobile-open={menu}>
      <a class="brand" href="#overview" onclick={() => navigate('Overview')}
        ><div class="brandmark"><Layers size={23} /></div>
        <div>ReturnScope<small>RETURNS WORKSPACE</small></div></a
      >
      <div class="workspace">
        <span class="avatar">R</span>
        <div>Robinbosky<small>Operations workspace</small></div>
        <ChevronDown size={14} />
      </div>
      <div class="nav-label">WORKSPACE</div>
      <nav>
        {#each nav as item}<button
            class:active={page === item.name}
            onclick={() => navigate(item.name)}
            ><item.icon size={19} />{item.name}{#if item.name === 'Claims' && summary?.raise}<span
                class="nav-count">{summary.raise}</span
              >{/if}</button
          >{/each}
      </nav>
      <div class="nav-label secondary-label">APPEARANCE</div>
      <nav>
        <button
          class="theme-toggle"
          aria-label={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
          onclick={() => changeTheme(theme === 'dark' ? 'light' : 'dark')}
          >{#if theme === 'dark'}<Sun size={18} />Light mode{:else}<Moon size={18} />Dark mode{/if}</button
        >
      </nav>
      <div class="sidebar-bottom">
        <div class="engine-note">
          <span class="live-dot"></span>{browserMode ? ' Browser processing active' : ' Backend processing active'}<small
            >{marketplace === 'amazon' ? 'Amazon' : 'Flipkart'} returns workspace</small
          >
        </div>
        <button class="profile" onclick={() => void logout()}
          ><span class="avatar light">A</span>
          <div>Admin<small>Workspace administrator</small></div>
          <LogOut size={16} /></button
        >
      </div>
    </aside>
    <div class="main-area">
      <header class="topbar">
        <button
          class="mobile-toggle icon-button"
          aria-label="Open navigation"
          onclick={() => (menu = !menu)}><Layers size={21} /></button
        >
        <div class="breadcrumb">Workspace <span>/</span><strong>{page}</strong></div>
        <div class="topbar-right">
          <span class="private-badge"><ShieldCheck size={14} />Private workspace</span><span
            class="avatar small">A</span
          >
        </div>
      </header>
      <main class="content">
        <div class="page-heading">
          <div>
            <div class="eyebrow">{marketplace.toUpperCase()} RETURNS INTELLIGENCE</div>
            <h1>
              {page === 'Overview'
                ? 'Every return. A clearer picture.'
                : page === 'Data imports'
                  ? 'Good data. Confident decisions.'
                  : page === 'Reports'
                    ? 'The detail behind every return.'
                    : 'Stay ahead of your claim window.'}
            </h1>
            <p>
              {page === 'Overview'
                ? 'Track your returns, reconcile payments, and recover what’s yours.'
                : page === 'Data imports'
                  ? 'Upload your three source files. We’ll take care of the reconciliation.'
                  : page === 'Claims'
                    ? marketplace === 'amazon'
                      ? 'Review missing returns with an outstanding refund debit.'
                      : 'Review Product Missing, DNE and SPF claim windows.'
                    : 'One source of truth for receipts, debits, and reimbursements.'}
            </p>
          </div>
          {#if page === 'Overview' || page === 'Reports'}<button
              class="primary"
              onclick={() => (page === 'Overview' ? navigate('Data imports') : exportFile('xlsx'))}
              >{#if page === 'Overview'}<Plus size={17} />Import data{:else}<ArrowDownToLine
                  size={16}
                />Export Excel{/if}</button
            >{/if}
        </div>
        <div class="filterbar">
          <div class="marketplace">
            {#if marketplace === 'amazon'}<span class="amazon-a">a<span>⌣</span></span
            >{:else}<span class="flipkart-f">F</span>{/if}<select
              aria-label="Marketplace"
              value={marketplace}
              onchange={(event) => void switchMarketplace((event.currentTarget as HTMLSelectElement).value as Marketplace)}
              ><option value="amazon">Amazon</option><option value="flipkart">Flipkart</option><option value="myntra" disabled>Myntra · Coming soon</option></select
            ><span class="pill success subtle">ACTIVE</span>
          </div>
          <div class="filter-divider"></div>
          <select
            aria-label="Year"
            bind:value={year}
            onchange={() => {
              month = '';
              pageNo = 1;
              void load();
            }}
            ><option value="">All years</option>{#each years as y}<option value={y}>{y}</option
              >{/each}</select
          ><select
            aria-label="Month"
            bind:value={month}
            onchange={() => {
              pageNo = 1;
              void load();
            }}
            ><option value="">All months</option
            >{#each filterMonths as m}<option
                value={m}>{monthLabel(m)}</option
              >{/each}</select
          >
          <div class="filter-spacer"></div>
          <span class="sync-label"
            ><span class="live-dot"></span>{busy ? 'Updating…' : 'Live from your imports'}</span
          ><button class="icon-button" aria-label="Refresh data" onclick={() => void load()}
            ><RefreshCw size={16} class={busy ? 'spin' : ''} /></button
          >
        </div>
        {#if browserMode}<div class="alert notice">
            <ShieldCheck size={17} />Your {marketplace === 'amazon' ? 'Amazon' : 'Flipkart'} files are processed in a separate private workspace on this device. No sample records are included and marketplace data is never mixed.
          </div>{/if}
        {#if error}<div class="alert error">
            <AlertCircle size={17} />{error}<button
              aria-label="Dismiss error"
              onclick={() => (error = '')}><X size={15} /></button
            >
          </div>{/if}{#if notice}<div class="alert notice">
            <CheckCircle2 size={17} />{notice}<button
              aria-label="Dismiss notice"
              onclick={() => (notice = '')}><X size={15} /></button
            >
          </div>{/if}
        {#if page === 'Overview'}
          <div class="section-heading">
            <h2>Returns at a glance</h2>
            <span>{month ? monthLabel(month) : year || 'All imported periods'}</span>
          </div>
          <div class="metric-grid clickable-metrics">
            {#each [
              { label: 'Returns', value: summary?.total, icon: Package, color: 'gray', kind: 'All returns' },
              { label: 'Received', value: summary?.received, icon: PackageCheck, color: 'blue', kind: 'Received' },
              { label: 'Not received', value: summary?.not_received, icon: PackageX, color: 'red', kind: 'Not Received' },
              { label: 'Debited', value: summary?.debited, icon: IndianRupee, color: 'red', kind: 'Debited' },
              { label: 'Not debited', value: summary?.not_debited, icon: ShieldCheck, color: 'blue', kind: 'Not Debited' }
            ] as m}<button class="metric-card metric-button" onclick={() => void openDashboardPreview(m.kind)}>
                <div class="metric-top">
                  <span>{m.label}</span>
                  <div class="metric-icon {m.color}"><m.icon size={18} /></div>
                </div>
                <strong>{number(m.value)}</strong><span class="view-link">Preview rows <ArrowUpRight size={13} /></span>
              </button>{/each}
          </div>
          <div class="dashboard-middle">
            <section class="panel returns-chart">
              <div class="panel-heading">
                <div>
                  <h2>Returns over time</h2>
                  <p>Physical receipt against reported returns</p>
                </div>
                <div class="legend">
                  <span><i class="green-bg"></i>Received</span><span
                    ><i class="orange-bg"></i>Not received</span
                  >
                </div>
              </div>
              {#if months.length}<div class="chart">
                  {#each months.slice(-12) as m}{@const s = overview!.monthly[m]}{@const maxTotal =
                      Math.max(...months.map((k) => overview!.monthly[k].total), 1)}
                    <div class="chart-column">
                      <div class="bar-label">{s.total}</div>
                      <div class="bar-space">
                        <div class="bar" style:height={`${(s.total / maxTotal) * 100}%`}>
                          <div
                            class="orange-bg"
                            style:height={`${(s.not_received / s.total) * 100}%`}
                          ></div>
                          <div
                            class="green-bg"
                            style:height={`${(s.received / s.total) * 100}%`}
                          ></div>
                        </div>
                      </div>
                      <span>{monthLabel(m).split(' ')[0]}</span>
                    </div>{/each}
                </div>{:else}<div class="empty-chart">
                  <Layers size={29} />
                  <h3>Your returns story starts here</h3>
                  <p>Import your {marketplace === 'amazon' ? 'Amazon' : 'Flipkart'} files to see your monthly picture.</p>
                  <button class="text-button" onclick={() => navigate('Data imports')}
                    >Upload your first files <ArrowRight size={15} /></button
                  >
                </div>{/if}
              <div class="chart-footer actions-only">
                <button
                  class="text-button"
                  onclick={() => navigate('Reports')}
                  >View reports <ArrowUpRight size={14} /></button
                >
              </div>
            </section>
            <section class="financial-panel">
              <div class="eyebrow">FINANCIAL SUMMARY</div>
              <h2>Exact financial position.</h2>
              <div class="finance-card-grid">
                <button onclick={() => void openDashboardPreview('Debited')}><span>Total refund</span><strong>{money(summary?.refund_debited)}</strong><small>Buyer refund debited</small></button>
                <button onclick={() => void openDashboardPreview('Debited')}><span>Recovered</span><strong class="recovered">{money(summary?.recovered)}</strong><small>{marketplace === 'amazon' ? 'Product reimbursement received' : 'SPF reimbursement received'}</small></button>
                <button onclick={() => void openDashboardPreview('Debited')}><span>Current product loss</span><strong class="loss-text">{money(summary?.loss)}</strong><small>Still unrecovered</small></button>
                <button onclick={() => void openDashboardPreview('Not Debited')}><span>No-loss cases</span><strong>{number(summary?.no_loss_cases)}</strong><small>Not debited or fully reimbursed</small></button>
              </div>
              <div class="finance-rows"><div><span>{marketplace === 'amazon' ? 'Fulfilment fee refunds' : 'Flipkart fee refunds'}</span><b>{money(summary?.fee_refund)}</b></div></div>
            </section>
          </div>
          <section class="panel financial-trend">
            <div class="panel-heading">
              <div><h2>Monthly financial loss</h2><p>Current product loss compared with actual product recovery</p></div>
              <div class="legend"><span><i class="red-bg"></i>Loss</span><span><i class="green-bg"></i>Recovered</span></div>
            </div>
            {#if months.length}<div class="money-chart">
              {#each months.slice(-12) as m}{@const ms = overview!.monthly[m]}{@const maxMoney = Math.max(...months.map((k) => Math.max(overview!.monthly[k].loss, overview!.monthly[k].recovered)), 1)}
                <div class="money-column"><div class="money-bars"><div class="money-bar loss-bar" style:height={`${(ms.loss / maxMoney) * 100}%`} title={'Loss ' + money(ms.loss)}></div><div class="money-bar recovery-bar" style:height={`${(ms.recovered / maxMoney) * 100}%`} title={'Recovered ' + money(ms.recovered)}></div></div><span>{monthLabel(m).split(' ')[0]}</span><small>{money(ms.loss)}</small></div>
              {/each}
            </div>{:else}<div class="empty-chart"><IndianRupee size={28}/><h3>No financial trend yet</h3><p>Import payment data to see loss and recovery by month.</p></div>{/if}
          </section>
          <div class="lower-grid">
            <section class="panel claims-overview">
              <div class="panel-heading">
                <div>
                  <h2>A little attention. A lot recovered.</h2>
                  <p>Your claims worklist, ready when you are.</p>
                </div>
                <div class="metric-icon red"><ShieldCheck size={20} /></div>
              </div>
              <div class="claim-metrics expanded-claims">
                {#each [
                  ['Raise Claim', summary?.raise, 'Ready to claim', 'red-bg'],
                  ['Scheduled', summary?.scheduled, 'Scheduled', 'blue-bg'],
                  ['Pending', summary?.claim_pending, 'Submitted · pending', 'blue-bg'],
                  ['Approved', summary?.claim_approved, 'Approved', 'blue-bg'],
                  ['Rejected', summary?.claim_rejected, 'Rejected', 'red-bg'],
                  ['Expired', summary?.expired, 'Expired · not raised', 'red-bg']
                ] as q}<button onclick={() => { navigate('Claims'); claim = String(q[0]); void load(); }}>
                    <strong>{number(q[1] as number)}</strong><span><i class={'dot ' + q[3]}></i>{q[2]}</span>
                  </button>{/each}
              </div>
              <div class="panel-footer actions-only">
                <button
                  class="text-button"
                  onclick={() => navigate('Claims')}>Open claims <ArrowRight size={15} /></button
                >
              </div>
            </section>
            <section class="panel data-health">
              <div class="panel-heading">
                <div>
                  <h2>Your data sources</h2>
                  <p>Three files. One complete picture.</p>
                </div>
                <button
                  class="icon-button"
                  aria-label="Manage data sources"
                  onclick={() => navigate('Data imports')}><ArrowUpRight size={18} /></button
                >
              </div>
              {#each cards as c}{@const i = imports.find(
                  (i) => i.kind === c.kind && i.active && i.status === 'completed'
                )}
                <div class="source-row">
                  <div class="source-icon {c.color}"><c.icon size={17} /></div>
                  <div>
                    <strong>{c.title}</strong><small
                      >{i ? i.filename : 'No file imported yet'}</small
                    >
                  </div>
                  <span class="pill {i ? 'success' : 'neutral'}">{i ? 'Ready' : 'Awaiting'}</span>
                </div>{/each}
            </section>
          </div>
          {#if summary?.unknown}<div class="alert notice">
              <AlertCircle size={17} />{number(summary.unknown)} cases need payment data or allocation
              review. They are kept separate from “Not Debited”.<button
                class="text-button"
                onclick={() => navigate('Reports')}>Review <ArrowRight size={14} /></button
              >
            </div>{/if}
        {:else if page === 'Data imports'}
          <section class="upload-intro">
            <div>
              <h2>Bring your records together</h2>
              <p>CSV, XLSX and ZIP archives · Up to 100 MB · Unsupported files are rejected</p>
            </div>
            <div class="coverage-input">
              <label for="coverage"
                >Physical month / workbook year <span>(if dates are missing)</span></label
              ><input id="coverage" type="month" bind:value={coverage} />
            </div>
          </section>
          <div class="upload-grid">
            {#each cards as c}<section class="upload-card">
                <div class="upload-card-icon {c.color}"><c.icon size={26} /></div>
                <h2>{c.title}</h2>
                <p>{c.desc}</p>
                <label class="dropzone"
                  ><CloudUpload size={27} /><strong
                    >{uploading === c.kind ? 'Uploading…' : 'Choose a file to upload'}</strong
                  ><span>CSV, Excel workbook or supported ZIP · multiple files allowed</span><input
                    aria-label={'Upload ' + c.title}
                    type="file"
                    multiple
                    accept=".csv,.xlsx,.zip"
                    disabled={!!uploading}
                    onchange={(e) => void upload(c.kind, e)}
                  /></label
                ><small><ShieldCheck size={13} />Duplicate file protection enabled</small>
              </section>{/each}
          </div>
          <div class="section-heading">
            <h2>Import history</h2>
            <button class="secondary" onclick={() => void reconcile()}
              ><RefreshCw size={15} />Reconcile all</button
            >
          </div>
          <section class="panel table-panel">
            <div class="table-scroll">
              <table>
                <thead
                  ><tr
                    ><th>Source file</th><th>Type</th><th>Uploaded</th><th>Rows</th><th>Status</th
                    ><th>Version</th><th></th></tr
                  ></thead
                ><tbody
                  >{#each imports as i}<tr
                      ><td
                        ><strong>{i.filename}</strong>{#if i.error}<small class="row-error"
                            >{i.error}</small
                          >{/if}</td
                      ><td class="capitalize">{i.kind}</td><td>{date(i.created_at)}</td><td
                        >{number(i.rows)}</td
                      ><td><span class="pill {tone(i.status)}">{i.status}</span></td><td
                        >{i.active ? 'Active' : 'Excluded'}</td
                      ><td><div class="row-actions"
                        >{#if i.status === 'completed'}<button
                            class="text-button"
                            disabled={deleting === i.id}
                            onclick={() => void activate(i)}
                            >{i.active ? 'Exclude' : 'Restore'}</button
                          >{/if}<button class="text-button danger" disabled={deleting === i.id} onclick={() => void removeImport(i)}
                          ><Trash2 size={13} />{deleting === i.id ? 'Deleting…' : 'Delete'}</button
                        ></div></td
                      ></tr
                    >{:else}<tr
                      ><td colspan="7" class="empty-cell"
                        >Your imported files and processing results will appear here.</td
                      ></tr
                    >{/each}</tbody
                >
              </table>
            </div>
          </section>
        {:else if page === 'Reports' || page === 'Claims'}
          <div class="report-totals">
            <span><b>{number(summary?.total)}</b> total cases</span><span
              ><b>{number(summary?.received)}</b> received</span
            ><span><b>{number(summary?.not_received)}</b> not received</span><span
              ><b>{number(summary?.not_debited)}</b> not debited</span
            ><span><b>{money(summary?.loss)}</b> remaining loss</span>
          </div>
          <section class="panel table-panel">
            <div class="tabs">
              {#if page === 'Reports'}{#each tabs as t}<button
                    class:active={tab === t}
                    onclick={() => {
                      tab = t;
                      pageNo = 1;
                      void load();
                    }}>{t}</button
                  >{/each}{:else}{#each ['Raise Claim', 'Scheduled', 'Pending', 'Approved', 'Partially Approved', 'Rejected', 'Expired'] as c}<button
                    class:active={(claim || 'Raise Claim') === c}
                    onclick={() => {
                      claim = c;
                      pageNo = 1;
                      void load();
                    }}>{c}</button
                  >{/each}{/if}
            </div>
            <div class="table-toolbar">
              <label class="searchbox"
                ><Search size={16} /><input
                  aria-label="Search order or SKU"
                  placeholder="Search order ID or SKU…"
                  bind:value={search}
                  oninput={() => {
                    pageNo = 1;
                    void load();
                  }}
                /></label
              ><select
                aria-label="Return status"
                bind:value={status}
                onchange={() => {
                  pageNo = 1;
                  void load();
                }}
                ><option value="">All return statuses</option
                >{#each ['Return Initiated', 'Returned to Seller', 'Delivered to Seller', 'In-transit to Seller Flex Node', 'Customer Rescheduled Pick-up', 'Return Has Been Abandoned'] as st}<option
                    >{st}</option
                  >{/each}</select
              ><select
                aria-label="Financial status"
                bind:value={financial}
                onchange={() => {
                  pageNo = 1;
                  void load();
                }}
                ><option value="">All payment statuses</option
                >{#each marketplace === 'flipkart' ? ['Debited', 'Not Debited', 'Already Reimbursed', 'Partially Reimbursed', 'Window Incomplete', 'Payment Not Found', 'Payment Review'] : ['Debited', 'Not Debited', 'Already Reimbursed', 'Partially Reimbursed', 'Payment Data Missing', 'Payment Review'] as f}<option
                    >{f}</option
                  >{/each}</select
              ><button class="secondary" onclick={() => exportFile('csv')}
                ><ArrowDownToLine size={15} />CSV</button
              ><button class="secondary" onclick={() => exportFile('xlsx')}
                ><FileSpreadsheet size={15} />Excel</button
              >
            </div>
            {#if tab === 'Summary' && page === 'Reports'}<div class="table-scroll">
                <table>
                  <thead
                    ><tr
                      ><th>Month</th><th>Total</th><th>Received</th><th>Not Received</th><th
                        >Debited</th
                      ><th>Not Debited</th><th>Review / Missing</th><th>Recovered</th><th
                        >Remaining loss</th
                      ></tr
                    ></thead
                  ><tbody
                    >{#each Object.keys(results?.monthly ?? {}).sort() as m}{@const s =
                        results!.monthly[m]}<tr
                        ><td>{monthLabel(m)}</td><td>{number(s.total)}</td><td
                          >{number(s.received)}</td
                        ><td>{number(s.not_received)}</td><td>{number(s.debited)}</td><td
                          >{number(s.not_debited)}</td
                        ><td>{number(s.unknown)}</td><td>{money(s.recovered)}</td><td
                          >{money(s.loss)}</td
                        ></tr
                      >{:else}<tr
                        ><td colspan="9" class="empty-cell"
                          >Import your files to see the monthly summary.</td
                        ></tr
                      >{/each}</tbody
                  >
                </table>
              </div>{:else}<div class="table-scroll">
                <table>
                  <thead
                    ><tr
                      ><th>Order / SKU</th><th>Period</th><th>Return status</th><th
                        >Physical receipt</th
                      ><th>Payment status</th><th>Remaining loss</th><th>System claim</th><th>Claim status</th><th></th></tr
                    ></thead
                  ><tbody
                    >{#each results?.rows ?? [] as c}<tr
                        ><td
                          ><strong class="order-id">{c.order_id}</strong><small>{c.sku}</small></td
                        ><td>{monthLabel(c.month)}</td><td class="status-text">{c.status}</td><td
                          ><span class="pill {tone(c.receipt)}">{c.receipt}</span
                          >{#if c.received_month}<small>{monthLabel(c.received_month)}</small
                            >{/if}</td
                        ><td><span class="pill {tone(c.financial)}">{c.financial}</span></td><td
                          class:loss-text={c.loss > 0}>{money(c.loss)}</td
                        ><td
                          ><span class="pill {tone(c.claim_status)}">{c.claim_status}</span
                          >{#if !c.claim_deadline.startsWith('0001')}<small>By {date(c.claim_deadline)}</small>{/if}</td
                        ><td><span class="pill {tone(c.user_claim_status || 'Not Raised')}">{c.user_claim_status || 'Not Raised'}</span>{#if c.claim_updated_date && !c.claim_updated_date.startsWith('0001')}<small>Updated {date(c.claim_updated_date)}</small>{/if}</td
                        ><td
                          ><button
                            class="icon-button"
                            aria-label={'View case ' + c.order_id}
                            onclick={() => void detail(c)}><ArrowUpRight size={17} /></button
                          ></td
                        ></tr
                      >{:else}<tr
                        ><td colspan="9" class="empty-cell"
                          ><Package size={30} />
                          <h3>
                            {imports.length
                              ? 'No cases match these filters'
                              : 'No returns imported yet'}
                          </h3>
                          <p>
                            {imports.length
                              ? 'Change the period or status to find your cases.'
                              : `Upload your ${marketplace === 'amazon' ? 'Amazon' : 'Flipkart'} data to start reconciling.`}
                          </p>
                          {#if !imports.length}<button
                              class="primary"
                              onclick={() => navigate('Data imports')}
                              >Import data <ArrowRight size={15} /></button
                            >{/if}</td
                        ></tr
                      >{/each}</tbody
                  >
                </table>
              </div>{/if}
            <div class="pagination">
              <span>{number(results?.total)} cases · 25 per page</span>
              <div>
                <button
                  class="icon-button"
                  aria-label="Previous page"
                  disabled={pageNo <= 1}
                  onclick={() => {
                    pageNo--;
                    void load();
                  }}><ChevronLeft size={17} /></button
                ><span>Page {pageNo}</span><button
                  class="icon-button"
                  aria-label="Next page"
                  disabled={pageNo * 25 >= (results?.total ?? 0)}
                  onclick={() => {
                    pageNo++;
                    void load();
                  }}><ChevronRight size={17} /></button
                >
              </div>
            </div>
          </section>
        {/if}
        <footer class="page-footer">
          <span><Layers size={13} /> ReturnScope <span>by Robinbosky</span></span><span
            >Made for clarity. Built for accountability.</span
          >
        </footer>
      </main>
    </div>
  </div>
  {#if dashboardPreview}<div class="modal-backdrop" role="presentation" onclick={(e) => { if (e.target === e.currentTarget) dashboardPreview = null; }}>
      <div class="preview-modal" role="dialog" aria-modal="true" aria-label="Dashboard data preview">
        <div class="panel-heading"><div><div class="eyebrow">DASHBOARD PREVIEW</div><h2>{dashboardPreview.title}</h2><p>{number(dashboardPreview.total)} exact cases for the selected marketplace and period.</p></div><button class="icon-button" aria-label="Close preview" onclick={() => (dashboardPreview = null)}><X size={21} /></button></div>
        <div class="table-scroll preview-table"><table><thead><tr><th>Order ID</th><th>SKU</th><th>Return status</th><th>Last updated</th><th>Physical</th><th>Financial</th><th>Refund</th><th>Recovered</th><th>Loss</th><th>System claim</th><th>Claim status</th><th></th></tr></thead><tbody>
          {#each dashboardPreview.rows as c}<tr><td><strong class="order-id">{c.order_id}</strong></td><td>{c.sku}</td><td>{c.status}</td><td>{date(c.updated)}</td><td><span class="pill {tone(c.receipt)}">{c.receipt}</span></td><td><span class="pill {tone(c.financial)}">{c.financial}</span></td><td>{money(Math.abs(c.refund_total))}</td><td>{money(c.recovered)}</td><td class:loss-text={c.loss > 0}>{money(c.loss)}</td><td><span class="pill {tone(c.claim_status)}">{c.claim_status}</span></td><td><span class="pill {tone(c.user_claim_status || 'Not Raised')}">{c.user_claim_status || 'Not Raised'}</span></td><td><button class="icon-button" onclick={() => { dashboardPreview = null; void detail(c); }} aria-label={'Open ' + c.order_id}><ArrowUpRight size={16} /></button></td></tr>
          {:else}<tr><td colspan="12" class="empty-cell">No rows match this card and filter.</td></tr>{/each}
        </tbody></table></div>
        {#if dashboardPreview.total > dashboardPreview.rows.length}<div class="panel-footer"><span>Showing first {number(dashboardPreview.rows.length)} of {number(dashboardPreview.total)} cases.</span><button class="text-button" onclick={() => { dashboardPreview=null; navigate('Reports'); }}>Open full report <ArrowRight size={14} /></button></div>{/if}
      </div>
    </div>{/if}
  {#if selected}<div
      class="modal-backdrop"
      role="presentation"
      onclick={(e) => {
        if (e.target === e.currentTarget) selected = null;
      }}
    >
      <div
        class="case-modal"
        role="dialog"
        aria-modal="true"
        aria-label="Return case detail"
        tabindex="-1"
        onkeydown={(e) => {
          if (e.key === 'Escape') selected = null;
        }}
      >
        <div class="panel-heading">
          <div>
            <div class="eyebrow">RETURN CASE</div>
            <h2>{selected.order_id}</h2>
            <p>{selected.sku}</p>
          </div>
          <button
            class="icon-button"
            aria-label="Close case detail"
            onclick={() => (selected = null)}><X size={21} /></button
          >
        </div>
        <div class="case-body">
          <div class="case-status">
            <span class="pill {tone(selected.receipt)}">{selected.receipt}</span><span
              class="pill {tone(selected.financial)}">{selected.financial}</span
            ><span class="pill {tone(selected.claim_status)}">System: {selected.claim_status}</span><span class="pill {tone(selected.user_claim_status || 'Not Raised')}">Claim: {selected.user_claim_status || 'Not Raised'}</span>
          </div>
          <dl>
            {#each [['Return month', monthLabel(selected.month)], ['Latest return status', selected.status], ['Last updated', date(selected.updated)], ['Delivered / received date', date(selected.delivered)], ['Refund date', date(selected.refund_date)], ['Order total', money(selected.order_total)], ['Refund total', money(selected.refund_total)], ['Fee refund', money(selected.fee_refund)], [marketplace === 'amazon' ? 'Reimbursement' : 'SPF reimbursement', money(selected.reimbursement)], ['Remaining product loss', money(selected.loss)], ['Claim rule', selected.claim_rule || '—'], ['Claim opens', date(selected.claim_open)], ['Claim deadline', date(selected.claim_deadline)], ['Claim raised date', date(selected.claim_raised_date)], ['Claim updated', date(selected.claim_updated_date)], [marketplace === 'amazon' ? 'Amazon Claim ID' : 'Flipkart SPF Claim ID', selected.amazon_claim_id || '—'], ['Requested amount', money(selected.claim_amount_requested)], ['Approved amount', money(selected.claim_amount_approved)], ['Rule version', selected.rule_version]] as pair}<div
              >
                <dt>{pair[0]}</dt>
                <dd>{pair[1]}</dd>
              </div>{/each}
          </dl>
          {#each selected.notes as n}<div class="alert notice">{n}</div>{/each}
          <section class="claim-editor">
            <div class="panel-heading"><div><h3>Manual claim tracking</h3><p>Permanent user action. New {marketplace === 'amazon' ? 'Amazon' : 'Flipkart'} imports cannot overwrite it.</p></div><span class="pill {tone(claimForm.status)}">{claimForm.status}</span></div>
            <div class="claim-form-grid">
              <label>Claim status<select bind:value={claimForm.status}>{#each ['Not Raised','Pending','Approved','Rejected','Partially Approved'] as cs}<option>{cs}</option>{/each}</select></label>
              <label>Claim raised date<input type="date" bind:value={claimForm.raisedDate} disabled={claimForm.status === 'Not Raised'} /></label>
              <label>{marketplace === 'amazon' ? 'Amazon Claim ID' : 'Flipkart SPF Claim ID'}<input bind:value={claimForm.amazonClaimID} placeholder="Optional claim ID" /></label>
              <label>Requested amount (₹)<input type="number" min="0" step="0.01" bind:value={claimForm.requested} /></label>
              <label>Approved amount (₹)<input type="number" min="0" step="0.01" bind:value={claimForm.approved} /></label>
              <label>Rejection reason<input bind:value={claimForm.rejectionReason} placeholder="Only if rejected" /></label>
              <label class="wide">Claim notes<textarea rows="3" bind:value={claimForm.notes} placeholder="Claim-specific notes"></textarea></label>
              <label class="wide">User notes<textarea rows="3" bind:value={claimForm.userNotes} placeholder="Internal operational notes"></textarea></label>
            </div>
            <div class="claim-save-row"><small>Only one current status is shown; every update is retained in history with date/time.</small><button class="primary" onclick={() => void saveClaim()}>Save claim update</button></div>
          </section>
          <h3>Source evidence</h3>
          <p class="muted">Every row used to explain this result.</p>
          {#each selected.evidence ?? [] as row}<div class="evidence">
              <span class="pill neutral">{row.kind}</span><strong
                >{row.type || row.status || 'Physical receipt'}</strong
              ><span>{date(row.date)}{row.kind === 'payment' ? ' · ' + money(row.amount) : ''}</span
              ><small>{row.source} · Row {row.line}</small>
            </div>{/each}
        </div>
      </div>
    </div>{/if}
{/if}
