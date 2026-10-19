// Scripts do painel. Extraidos do base.html do CtrlLicenca e servidos como
// estatico, para que o template fique so com marcacao.

// ============================================================
// Tema
// ============================================================

// O icone e um <use> do sprite SVG. Trocar o href mantem o traco monocromatico
// herdando currentColor -- escrever textContent aqui apagaria o SVG e voltaria
// ao emoji colorido.
function aplicarIconeTema(tema) {
  const use = document.querySelector('#theme-btn use');
  if (use) use.setAttribute('href', tema === 'dark' ? '/static/icons.svg#moon' : '/static/icons.svg#sun');
}

function toggleTheme() {
  const html = document.documentElement;
  const next = html.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
  html.setAttribute('data-theme', next);
  localStorage.setItem('theme', next);
  aplicarIconeTema(next);
}

aplicarIconeTema(localStorage.getItem('theme') || 'dark');

// ============================================================
// Sidebar
// ============================================================

(function () {
  const sidebar = document.getElementById('sidebar');
  const backdrop = document.getElementById('sidebar-backdrop');
  const toggle = document.getElementById('sidebar-toggle');
  if (!sidebar) return;

  function open() {
    sidebar.classList.add('open');
    backdrop?.classList.add('show');
    toggle?.classList.add('is-open');
  }
  function close() {
    sidebar.classList.remove('open');
    backdrop?.classList.remove('show');
    toggle?.classList.remove('is-open');
  }

  toggle?.addEventListener('click', () => (sidebar.classList.contains('open') ? close() : open()));
  backdrop?.addEventListener('click', close);
  document.querySelectorAll('.nav-item').forEach((a) => a.addEventListener('click', close));
})();

// ============================================================
// Dialogs
// ============================================================

document.addEventListener('click', function (e) {
  const abrir = e.target.closest('[data-open-dialog]');
  if (abrir) document.getElementById(abrir.getAttribute('data-open-dialog'))?.showModal();
  const fechar = e.target.closest('[data-close-dialog]');
  if (fechar) fechar.closest('dialog')?.close();
});

// ============================================================
// Tabelas: filtro e ordenacao
// ============================================================

function initTableFilter(cfg) {
  const tbody = document.querySelector('#' + cfg.tableId + ' tbody');
  if (!tbody) return;

  const searchEl = cfg.searchId ? document.getElementById(cfg.searchId) : null;
  const filterEls = (cfg.filters || [])
    .map((f) => ({ el: document.getElementById(f.id), col: f.col }))
    .filter((f) => f.el);

  function dataRows() {
    return Array.from(tbody.querySelectorAll('tr')).filter((r) => r.cells.length > 1);
  }

  function apply() {
    const query = searchEl?.value.trim().toLowerCase() || '';
    let count = 0;
    dataRows().forEach((row) => {
      let show = true;
      if (query) {
        const text = Array.from(row.cells).map((c) => c.textContent.trim()).join(' ').toLowerCase();
        if (!text.includes(query)) show = false;
      }
      if (show) {
        for (const { el, col } of filterEls) {
          if (!el.value) continue;
          const cellText = (row.cells[col]?.textContent || '').trim().toLowerCase();
          if (!cellText.includes(el.value.toLowerCase())) {
            show = false;
            break;
          }
        }
      }
      // Cada filtro guarda sua decisão num data-attribute e a linha só
      // aparece quando os dois concordam. Escrever display direto aqui faria
      // o filtro de texto apagar o de estado, e vice-versa.
      row.dataset.filtroTexto = show ? 'ok' : 'nao';
      const visivel = show && row.dataset.filtroEstado !== 'nao';
      row.style.display = visivel ? '' : 'none';
      if (visivel) count++;
    });

    const noRes = cfg.noResultsId ? document.getElementById(cfg.noResultsId) : null;
    const rows = dataRows();
    if (noRes) noRes.style.display = rows.length > 0 && count === 0 ? 'block' : 'none';
    const counter = cfg.countId ? document.getElementById(cfg.countId) : null;
    if (counter) counter.textContent = rows.length > 0 ? count + ' de ' + rows.length : '';
  }

  searchEl?.addEventListener('input', apply);
  filterEls.forEach(({ el }) => el.addEventListener('change', apply));
}

function initTableSort(tableId) {
  const table = document.getElementById(tableId);
  if (!table) return;

  let sortCol = -1;
  let sortAsc = true;

  table.querySelectorAll('thead th[data-sort]').forEach((th) => {
    th.addEventListener('click', function () {
      const col = parseInt(this.dataset.sort, 10);
      sortAsc = sortCol === col ? !sortAsc : true;
      sortCol = col;

      table.querySelectorAll('thead th[data-sort] .sort-ico').forEach((i) => (i.textContent = '↕'));
      const ico = this.querySelector('.sort-ico');
      if (ico) ico.textContent = sortAsc ? '↑' : '↓';

      const tbody = table.querySelector('tbody');
      const rows = Array.from(tbody.querySelectorAll('tr')).filter((r) => r.cells.length > 1);
      rows.sort((a, b) => {
        const av = (a.cells[col]?.textContent || '').trim().toLowerCase();
        const bv = (b.cells[col]?.textContent || '').trim().toLowerCase();
        return sortAsc ? av.localeCompare(bv, 'pt') : bv.localeCompare(av, 'pt');
      });
      rows.forEach((r) => tbody.appendChild(r));
    });
  });
}

// ============================================================
// CSRF
// ============================================================

// Token do processo, exigido em POST e na query de SSE/WebSocket.
const CSRF = document.querySelector('meta[name="csrf-token"]')?.content || '';

/** Anexa o token CSRF a uma URL de stream (SSE/WS). */
function comCsrf(url) {
  return url + (url.includes('?') ? '&' : '?') + 'csrf=' + encodeURIComponent(CSRF);
}

// ============================================================
// Menu retrátil
// ============================================================

// O estado inicial já foi aplicado no <head>, antes da primeira pintura.
// Aqui só tratamos o clique.
(function () {
  const btn = document.getElementById('colapsar');
  if (!btn) return;

  function sincronizarTitulo() {
    const retraida = document.documentElement.classList.contains('sidebar-retraida');
    btn.title = retraida ? 'Expandir menu' : 'Recolher menu';
    btn.setAttribute('aria-label', btn.title);
  }

  btn.addEventListener('click', () => {
    const retraida = document.documentElement.classList.toggle('sidebar-retraida');
    localStorage.setItem('sidebar', retraida ? 'retraida' : 'expandida');
    sincronizarTitulo();
  });

  sincronizarTitulo();
})();

// ============================================================
// Aviso temporário
// ============================================================

let _toastTimer;
function toast(msg) {
  let el = document.querySelector('.toast');
  if (!el) {
    el = document.createElement('div');
    el.className = 'toast';
    document.body.appendChild(el);
  }
  el.textContent = msg;
  el.classList.add('visivel');
  clearTimeout(_toastTimer);
  _toastTimer = setTimeout(() => el.classList.remove('visivel'), 1800);
}

// ============================================================
// Copiar para a área de transferência
// ============================================================

// Alternativa ao navigator.clipboard, que exige contexto seguro e ativação
// do usuário. Quando ele não está disponível ou não resolve, um textarea
// temporário com execCommand ainda dá conta.
function copiarLegado(texto) {
  const ta = document.createElement('textarea');
  ta.value = texto;
  ta.setAttribute('readonly', '');
  ta.style.cssText = 'position:fixed;top:-1000px;opacity:0';
  document.body.appendChild(ta);
  ta.select();
  let ok = false;
  try { ok = document.execCommand('copy'); } catch { ok = false; }
  ta.remove();
  return ok;
}

document.addEventListener('click', async function (e) {
  const btn = e.target.closest('[data-copiar]');
  if (!btn) return;
  e.preventDefault();

  const texto = btn.dataset.copiar;
  let copiou = false;

  if (navigator.clipboard?.writeText) {
    try {
      // A API pode ficar pendente para sempre quando falta ativação do
      // usuário. Sem este limite, nenhum dos dois ramos roda e o usuário
      // não recebe retorno nenhum.
      await Promise.race([
        navigator.clipboard.writeText(texto),
        new Promise((_, rej) => setTimeout(() => rej(new Error('timeout')), 1200)),
      ]);
      copiou = true;
    } catch {
      copiou = false;
    }
  }

  if (!copiou) copiou = copiarLegado(texto);
  toast(copiou ? 'Copiado: ' + texto : 'Não foi possível copiar.');
});

// ============================================================
// Filtros rápidos por estado
// ============================================================

function initChipsEstado(cfg) {
  const tbody = document.querySelector('#' + cfg.tableId + ' tbody');
  const chips = document.querySelectorAll('[data-estado]');
  if (!tbody || !chips.length) return;

  function aplicar(estado) {
    tbody.querySelectorAll('tr[data-estado]').forEach((tr) => {
      const combina =
        estado === 'todos' ||
        (estado === 'running' && tr.dataset.estado === 'running') ||
        (estado === 'parado' && tr.dataset.estado !== 'running');
      // Reutiliza o mesmo display:none do filtro de texto; os dois se somam
      // porque cada um guarda sua decisão num data-attribute.
      tr.dataset.filtroEstado = combina ? 'ok' : 'nao';
      tr.style.display = combina && tr.dataset.filtroTexto !== 'nao' ? '' : 'none';
    });
  }

  chips.forEach((c) => {
    c.addEventListener('click', () => {
      chips.forEach((o) => o.classList.remove('ativo'));
      c.classList.add('ativo');
      aplicar(c.dataset.estado);
    });
  });
}

// ============================================================
// Atalhos de teclado
// ============================================================

document.addEventListener('keydown', function (e) {
  const digitando = /^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement?.tagName);

  // "/" foca o campo de busca — convenção de GitHub, Slack e afins.
  if (e.key === '/' && !digitando) {
    const busca = document.querySelector('input[type="search"]');
    if (busca) { e.preventDefault(); busca.focus(); }
  }

  // Esc limpa a busca e devolve o foco à página.
  if (e.key === 'Escape' && document.activeElement?.type === 'search') {
    document.activeElement.value = '';
    document.activeElement.dispatchEvent(new Event('input'));
    document.activeElement.blur();
  }

  // "b" recolhe/expande o menu.
  if (e.key === 'b' && !digitando && !e.ctrlKey && !e.metaKey) {
    document.getElementById('colapsar')?.click();
  }
});

// ============================================================
// Atualização automática
// ============================================================

// Recarrega a página em intervalo fixo enquanto a aba estiver visível.
// É o degrau antes do SSE (fase 4 do plano): resolve o essencial — ver o
// estado mudar sem apertar F5 — sem abrir conexão persistente.
function initAutoRefresh(segundos) {
  const chk = document.getElementById('auto-refresh');
  if (!chk) return;

  let timer = null;
  const ligado = localStorage.getItem('autorefresh') === '1';
  chk.checked = ligado;

  function parar()  { if (timer) { clearInterval(timer); timer = null; } }
  function iniciar() {
    parar();
    timer = setInterval(() => {
      // Não recarrega com um diálogo aberto nem com a aba em segundo plano.
      if (document.hidden) return;
      if (document.querySelector('dialog[open]')) return;
      if (document.activeElement?.type === 'search' && document.activeElement.value) return;
      location.reload();
    }, segundos * 1000);
  }

  chk.addEventListener('change', () => {
    localStorage.setItem('autorefresh', chk.checked ? '1' : '0');
    chk.checked ? iniciar() : parar();
  });

  if (ligado) iniciar();
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) parar();
    else if (chk.checked) iniciar();
  });
}

// ============================================================
// Logs ao vivo (SSE)
// ============================================================

function initLogStream(cfg) {
  const visor    = document.getElementById('visor');
  const estado   = document.getElementById('log-estado');
  const contagem = document.getElementById('log-contagem');
  const filtro   = document.getElementById('filtro-log');
  const tail     = document.getElementById('tail');
  const optTs    = document.getElementById('opt-timestamps');
  const optWrap  = document.getElementById('opt-wrap');
  const optFim   = document.getElementById('opt-scroll');
  if (!visor) return;

  // Limite de linhas em memória. Um container tagarela encheria o DOM e
  // travaria a aba em poucos minutos sem este teto.
  const MAX = 5000;
  let fonte = null;
  let total = 0;

  function marcarEstado(txt, classe) {
    estado.textContent = txt;
    estado.className = 'log-estado ' + (classe || '');
  }

  function visivel(linha) {
    const q = filtro.value.trim().toLowerCase();
    return !q || linha.textContent.toLowerCase().includes(q);
  }

  function acrescentar(ev) {
    const linha = document.createElement('div');
    linha.className = 'log-linha ' + (ev.f === 'err' ? 'log-err' : 'log-out');
    linha.textContent = ev.t;
    if (!visivel(linha)) linha.style.display = 'none';

    visor.appendChild(linha);
    total++;

    while (visor.childElementCount > MAX) visor.removeChild(visor.firstElementChild);
    contagem.textContent = total;

    if (optFim.checked) visor.scrollTop = visor.scrollHeight;
  }

  function conectar() {
    if (fonte) fonte.close();
    visor.innerHTML = '';
    total = 0;
    contagem.textContent = '0';
    marcarEstado('conectando…');

    const params = new URLSearchParams({ tail: tail.value });
    if (optTs.checked) params.set('timestamps', '1');

    // O token CSRF vai na query: o EventSource não permite cabeçalhos.
    fonte = new EventSource(comCsrf('/api/containers/' + encodeURIComponent(cfg.id) + '/logs/stream?' + params));

    fonte.onopen    = () => marcarEstado('ao vivo', 'log-ok');
    fonte.onmessage = (e) => { try { acrescentar(JSON.parse(e.data)); } catch {} };
    fonte.addEventListener('fim',  () => { marcarEstado('encerrado', 'log-off'); fonte.close(); });
    fonte.addEventListener('erro', (e) => { marcarEstado('erro: ' + e.data, 'log-off'); fonte.close(); });
    fonte.onerror   = () => marcarEstado('reconectando…', 'log-off');
  }

  filtro.addEventListener('input', () => {
    let vis = 0;
    for (const l of visor.children) {
      const ok = visivel(l);
      l.style.display = ok ? '' : 'none';
      if (ok) vis++;
    }
    contagem.textContent = filtro.value ? vis + ' de ' + total : total;
  });

  tail.addEventListener('change', conectar);
  optTs.addEventListener('change', conectar);
  optWrap.addEventListener('change', () => visor.classList.toggle('wrap', optWrap.checked));

  document.getElementById('btn-limpar').addEventListener('click', () => {
    visor.innerHTML = '';
    total = 0;
    contagem.textContent = '0';
  });

  document.getElementById('btn-baixar').addEventListener('click', () => {
    const texto = [...visor.children].map((l) => l.textContent).join('\n');
    const url = URL.createObjectURL(new Blob([texto], { type: 'text/plain;charset=utf-8' }));
    const a = document.createElement('a');
    a.href = url;
    a.download = cfg.nome + '-' + new Date().toISOString().slice(0, 19).replace(/[:T]/g, '') + '.log';
    a.click();
    URL.revokeObjectURL(url);
  });

  // Fecha o stream ao sair da página: sem isto o servidor segue lendo do
  // daemon para um cliente que já foi embora.
  window.addEventListener('beforeunload', () => fonte?.close());

  conectar();
}

// ============================================================
// Baixar imagem, com progresso
// ============================================================

function initPullImagem() {
  const dlg     = document.getElementById('dlg-pull');
  const ref     = document.getElementById('ref');
  const iniciar = document.getElementById('pull-iniciar');
  const fechar  = document.getElementById('pull-fechar');
  const painel  = document.getElementById('pull-progresso');
  const fill    = document.getElementById('pull-fill');
  const log     = document.getElementById('pull-log');
  if (!dlg || !iniciar) return;

  let abortar = null;

  function linha(txt) {
    log.textContent += txt + '\n';
    log.scrollTop = log.scrollHeight;
  }

  iniciar.addEventListener('click', async () => {
    const imagem = ref.value.trim();
    if (!imagem) { ref.focus(); return; }

    painel.hidden = false;
    log.textContent = '';
    fill.style.width = '0%';
    iniciar.disabled = true;
    ref.disabled = true;
    fechar.textContent = 'Cancelar';

    const ctrl = new AbortController();
    abortar = ctrl;

    // Progresso por camada: o Docker emite uma linha por camada, com id
    // próprio. Contar camadas concluídas dá uma barra honesta, que não
    // volta para trás quando uma camada nova aparece no meio.
    const camadas = new Map();
    function atualizarBarra() {
      if (!camadas.size) return;
      const prontas = [...camadas.values()].filter((s) => /complete|exists/i.test(s)).length;
      fill.style.width = Math.round((prontas / camadas.size) * 100) + '%';
    }

    try {
      const corpo = new URLSearchParams({ csrf: CSRF, ref: imagem });
      const resp = await fetch('/images/pull', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: corpo,
        signal: ctrl.signal,
      });
      if (!resp.ok) throw new Error('HTTP ' + resp.status + ': ' + (await resp.text()));

      // NDJSON: um objeto por linha, conforme o download avança.
      const reader = resp.body.getReader();
      const dec = new TextDecoder();
      let resto = '';

      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        resto += dec.decode(value, { stream: true });
        const partes = resto.split('\n');
        resto = partes.pop();

        for (const p of partes) {
          if (!p.trim()) continue;
          let ev; try { ev = JSON.parse(p); } catch { continue; }

          if (ev.erro) throw new Error(ev.erro);
          if (ev.fim) {
            fill.style.width = '100%';
            linha('Concluído: ' + ev.fim);
            toast('Imagem baixada: ' + ev.fim);
            setTimeout(() => location.reload(), 900);
            return;
          }
          if (ev.id) { camadas.set(ev.id, ev.status || ''); atualizarBarra(); }
          linha((ev.id ? ev.id.slice(0, 12) + '  ' : '') + (ev.status || '') + (ev.prog ? '  ' + ev.prog : ''));
        }
      }
    } catch (e) {
      if (e.name === 'AbortError') { linha('Cancelado.'); }
      else { linha('ERRO: ' + e.message); toast('Falha ao baixar a imagem.'); }
      fill.style.width = '0%';
    } finally {
      abortar = null;
      iniciar.disabled = false;
      ref.disabled = false;
      fechar.textContent = 'Fechar';
    }
  });

  // Fechar durante o download aborta de verdade, em vez de deixar o
  // request órfão consumindo banda.
  fechar.addEventListener('click', () => abortar?.abort());
  dlg.addEventListener('close', () => abortar?.abort());

  ref.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { e.preventDefault(); iniciar.click(); }
  });

  // Atalhos de imagens comuns.
  dlg.querySelectorAll('[data-sugestao]').forEach((b) => {
    b.addEventListener('click', () => {
      ref.value = b.dataset.sugestao;
      ref.focus();
      atualizarLinkHub();
    });
  });

  // O link do Docker Hub acompanha o que está digitado: busca o termo em vez
  // de abrir a home, que obrigaria o usuário a digitar tudo de novo lá.
  const linkHub = document.getElementById('link-hub');
  function atualizarLinkHub() {
    if (!linkHub) return;
    // Só o nome da imagem interessa na busca; a tag e o registro, não.
    const termo = ref.value.trim().split(':')[0].split('/').pop();
    linkHub.href = 'https://hub.docker.com/search?q=' + encodeURIComponent(termo);
  }
  ref.addEventListener('input', atualizarLinkHub);
  atualizarLinkHub();

  // Foca o campo ao abrir, para já poder digitar.
  dlg.addEventListener('close', () => { painel.hidden = true; });
  new MutationObserver(() => { if (dlg.open) ref.focus(); })
    .observe(dlg, { attributes: true, attributeFilter: ['open'] });
}
