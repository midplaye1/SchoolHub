import { api, query } from './api.js';
import { esc, icon, icons, number, date, localDay, toast, empty, pagination, bookCover, status } from './components/ui.js?v=warm-library';

const content = document.getElementById('content');
const dialog = document.getElementById('modal');
const modalContent = document.getElementById('modal-content');
const pageNames = { dashboard:'Обзор библиотеки', books:'Каталог книг', readers:'Читатели', loans:'Выдачи и возвраты', stats:'Статистика', accounts:'Пользователи' };
const palette = ['#a45e49','#859076','#c8a472','#6e898a','#bf8d7d','#aaa08c','#8c7660','#d1bdaa'];
let catalogView='shelf';
try { catalogView=localStorage.getItem('libraryhub-catalog-view')||'shelf'; } catch {}
let page = 'dashboard', pageNumber = 1, loanTab = 'active', renderID = 0, modalID = 0;
let filters = {}, records = new Map(), charts = [];
let currentUser = null;
const isReader = () => currentUser?.role === 'reader';
let activeTheme;
try { activeTheme = localStorage.getItem('libraryhub-theme'); } catch {}
if (activeTheme === 'dark') document.documentElement.dataset.theme = 'dark';

function updateThemeButton() {
 const dark = document.documentElement.dataset.theme === 'dark';
 document.getElementById('theme').innerHTML = `${icon(dark?'sun':'moon')}<span>${dark?'Светлая':'Тёмная'} тема</span><span class="switch-track"></span>`;
 icons();
}
updateThemeButton();
document.getElementById('theme').onclick = () => {
 const theme = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
 document.documentElement.dataset.theme = theme;
 try { localStorage.setItem('libraryhub-theme', theme); } catch {}
 updateThemeButton(); charts.forEach(chart => {
   const c = theme === 'dark' ? '#b4a493' : '#897b6b';
   if (chart.options.scales?.x) chart.options.scales.x.ticks.color = c;
   if (chart.options.scales?.y) chart.options.scales.y.ticks.color = c;
   chart.update();
 });
};
document.getElementById('today').textContent = new Date().toLocaleDateString('ru-RU', {day:'numeric',month:'long',year:'numeric'});
function closeSidebar() { document.getElementById('sidebar').classList.remove('open'); document.getElementById('shade').classList.remove('open'); }
document.getElementById('menu').onclick = () => { document.getElementById('sidebar').classList.toggle('open'); document.getElementById('shade').classList.toggle('open'); };
document.getElementById('shade').onclick = closeSidebar;

function heading(title, subtitle, actions = '', eyebrow = '') {
 return `<div class="page-heading"><div>${eyebrow?`<div class="eyebrow">${eyebrow}</div>`:''}<h1>${title}</h1><p>${subtitle}</p></div><div class="heading-actions">${actions}</div></div>`;
}
const addButton = (action, label, symbol = 'plus') => `<button class="button primary" data-action="${action}">${icon(symbol)}${label}</button>`;
function skeleton() {
 return `<div class="skeleton skeleton-heading"></div><div class="skeleton skeleton-sub"></div><div class="stat-grid">${Array(4).fill('<div class="skeleton skeleton-card"></div>').join('')}</div><div class="skeleton skeleton-panel"></div>`;
}
function statCard(label, value, symbol, color, note, alert = false) {
 return `<div class="stat-card"><div class="stat-top"><span>${label}</span><span class="stat-icon ${color}">${icon(symbol)}</span></div><div class="stat-value">${number(value)}</div><div class="stat-bottom ${alert?'alert':''}">${alert?icon('circle-alert'):'<span class="dot"></span>'}${note}</div></div>`;
}
function cards(s) {
 return `<div class="stat-grid">${statCard('Всего книг',s.books,'book-open','','Наименований в каталоге')}${statCard('Доступно экземпляров',s.available,'book-check','green','Готовы к выдаче читателям')}${statCard('Сейчас на руках',s.active,'arrow-left-right','orange','Активных выдач')}${statCard('Просроченные выдачи',s.overdue,'clock-3','red',s.overdue?'Требуют вашего внимания':'Все книги возвращают вовремя',s.overdue>0)}</div>`;
}
function loanRows(items, actions = true) {
 return items.map(l => `<tr class="${l.overdue?'overdue-row':''}"><td><button class="row-title book-cell" data-action="view-book" data-id="${l.book_id}">${bookCover(l.book_title,l.book_id,true)}<span><strong>${esc(l.book_title)}</strong><small>${esc(l.book_author)}</small></span></button></td><td><button class="row-title reader-cell" data-action="view-reader" data-id="${l.reader_id}"><span class="reader-avatar">${esc(initials(l.reader_name))}</span><span><strong>${esc(l.reader_name)}</strong><small>${esc(l.reader_class)} класс</small></span></button></td><td class="text-muted">${date(l.issued_at)}</td><td class="${l.overdue?'text-red':'text-muted'}">${date(l.due_at)}</td><td>${status(l)}</td>${actions?`<td>${l.returned_at?`<span class="text-muted">${date(l.returned_at)}</span>`:`<button class="button small" data-action="return-loan" data-id="${l.id}">${icon('corner-down-left')}Вернуть</button>`}</td>`:''}</tr>`).join('');
}
function loanTable(items, actions = true, recent = false) {
 if (!items.length) return empty('Выдач пока нет', 'Здесь появятся книги, выданные читателям.');
 return `<div class="table-wrap ${recent?'recent-table':''}"><table><thead><tr><th>Книга</th><th>Читатель</th><th>Дата выдачи</th><th>Вернуть до</th><th>Статус</th>${actions?'<th>Возврат</th>':''}</tr></thead><tbody>${loanRows(items,actions)}</tbody></table></div>`;
}
function initials(name) { return name.trim().split(/\s+/).slice(0,2).map(p=>p[0]).join(''); }
function popularPanel(items) {
 return `<section class="panel"><div class="panel-head"><div><h2>Популярные книги</h2><p class="panel-subtitle">Пятёрка читательских фаворитов</p></div>${icon('trending-up','text-muted')}</div><div class="popular-list">${items.length?items.map((b,i)=>`<div class="popular-item"><span class="rank">0${i+1}</span>${bookCover(b.label,i+1)}<div class="popular-info"><strong title="${esc(b.label)}">${esc(b.label)}</strong><small>${esc(b.author)}</small></div><div class="popular-count">${number(b.value)}<small>выдач</small></div></div>`).join(''):empty('Пока нет выдач','Популярность рассчитывается по истории выдач.')}</div></section>`;
}
function chartPanel(id, title = 'Динамика выдач', subtitle = 'Активность читателей за последние 6 месяцев') {
 return `<section class="panel"><div class="panel-head"><div><h2>${title}</h2><p class="panel-subtitle">${subtitle}</p></div><span class="period">${icon('calendar-days')}6 месяцев</span></div><div class="chart-wrap"><canvas id="${id}" role="img" aria-label="График выдач книг по месяцам"></canvas></div></section>`;
}
function genresPanel(genres, total) {
 return `<section class="panel"><div class="panel-head"><div><h2>Книги по жанрам</h2><p class="panel-subtitle">Разнообразие вашей библиотеки</p></div>${icon('chart-pie','text-muted')}</div><div class="genre-layout"><div class="donut-wrap"><canvas id="genre-chart" role="img" aria-label="Распределение книг по жанрам"></canvas><div class="donut-label">${number(total)}<small>наименований</small></div></div><div class="genre-list">${genres.map((g,i)=>`<div class="genre-row"><span style="background:${palette[i%palette.length]}"></span>${esc(g.label)}<b>${number(g.value)}</b></div>`).join('') || '<p>Нет данных</p>'}</div></div></section>`;
}
async function dashboard(id, statistics = false) {
 const [s,popular,genres,months,extra] = await Promise.all([api('/stats/overview'),api('/stats/popular-books'),api('/stats/genres'),api('/stats/loans-by-month'),api(statistics?'/stats/popular-genres':'/loans?limit=5')]);
 if (id!==renderID) return;
 document.getElementById('loan-count').textContent = s.active; document.getElementById('loan-count').hidden = !s.active;
 content.innerHTML = heading(statistics?'Статистика библиотеки':'Обзор библиотеки',statistics?'Цифры, которые помогают лучше понимать ваших читателей.':'Хороший день, чтобы открыть новую историю.',statistics?'':`${'<a class="button" href="#loans">'}${icon('arrow-left-right')}Выдать книгу</a>${addButton('add-book','Добавить книгу')}`,statistics?'АНАЛИТИКА':'ВАША БИБЛИОТЕКА СЕГОДНЯ') + cards(s) +
 (statistics?`<div class="stats-extra"><div class="panel mini-stat"><span class="stat-icon">${icon('library-big')}</span><div><small>Всего экземпляров</small><strong>${number(s.copies)}</strong></div></div><div class="panel mini-stat"><span class="stat-icon green">${icon('users-round')}</span><div><small>Зарегистрировано читателей</small><strong>${number(s.readers)}</strong></div></div><div class="panel mini-stat"><span class="stat-icon orange">${icon('calendar-check')}</span><div><small>Выдач в текущем месяце</small><strong>${number(s.this_month)}</strong></div></div></div>`:'') +
 `<div class="dashboard-grid">${chartPanel('loans-chart')}${popularPanel(popular)}</div><div class="dashboard-grid"><div class="panel"><div class="panel-head"><div><h2>${statistics?'Популярные жанры':'Последние выдачи'}</h2><p class="panel-subtitle">${statistics?'По количеству выдач за всё время':'Что читают прямо сейчас'}</p></div>${statistics?'':`<a class="text-link" href="#loans">Все выдачи ${icon('arrow-up-right')}</a>`}</div>${statistics?`<div class="chart-wrap"><canvas id="popular-genres-chart" role="img" aria-label="Популярность жанров по количеству выдач"></canvas></div>`:loanTable(extra.items,false,true)}</div>${genresPanel(genres,s.books)}</div>`+
 (statistics?'':`<section class="panel quick-actions"><span>Быстрые действия</span><button class="button" data-action="add-book">${icon('book-plus')}Добавить книгу</button><button class="button" data-action="issue-loan">${icon('arrow-up-right')}Выдать книгу</button><button class="button" data-action="add-reader">${icon('user-plus')}Добавить читателя</button></section>`);
 // The heading shortcut opens the issuance dialog directly.
 if (!statistics) content.querySelector('.heading-actions a').onclick = event => { event.preventDefault(); openLoan(); };
 drawMonths(months);
 drawGenres(genres);
 if(statistics) drawPopularGenres(extra);
 icons();
}
function chartOptions() {
 const dark = document.documentElement.dataset.theme==='dark';
 return { responsive:true, maintainAspectRatio:false, plugins:{legend:{display:false},tooltip:{backgroundColor:dark?'#e8edf6':'#20283e',titleColor:dark?'#0f172a':'#fff',bodyColor:dark?'#0f172a':'#fff',padding:11,displayColors:false}},scales:{x:{grid:{display:false},border:{display:false},ticks:{color:dark?'#b4a493':'#897b6b',font:{family:'Inter',size:9}}},y:{beginAtZero:true,border:{display:false,dash:[4,4]},grid:{color:dark?'#3d342c':'#eae3d9',tickLength:0},ticks:{precision:0,padding:12,color:dark?'#b4a493':'#897b6b',font:{family:'Inter',size:9}}}}};
}
function drawMonths(items) {
 const canvas = document.getElementById('loans-chart');
 const labels = items.map(x=>new Date(`${x.label}-02`).toLocaleDateString('ru-RU',{month:'short'}));
 if (!window.Chart) { fallbackBars(canvas,items.map((x,i)=>({...x,label:labels[i]}))); return; }
 const gradient = canvas.getContext('2d').createLinearGradient(0,0,0,250); gradient.addColorStop(0,'#b8755830'); gradient.addColorStop(1,'#b8755800');
 charts.push(new Chart(canvas,{type:'line',data:{labels,datasets:[{label:'Выдач',data:items.map(x=>x.value),borderColor:'#a45e49',backgroundColor:gradient,borderWidth:2.5,tension:.4,fill:true,pointRadius:3,pointBorderWidth:2,pointBackgroundColor:'#fff',pointBorderColor:'#a45e49',pointHoverRadius:5}]},options:chartOptions()}));
}
function drawGenres(items) {
 const canvas = document.getElementById('genre-chart');
 if (!window.Chart) {
   const total = items.reduce((s,x)=>s+x.value,0); let cursor=0;
   const stops = items.map((x,i)=>{const start=cursor;cursor+=total?x.value/total*100:0;return `${palette[i%palette.length]} ${start}% ${cursor}%`;});
   const ring=document.createElement('div');ring.style.cssText=`width:100%;height:100%;border-radius:50%;background:conic-gradient(${stops.join(',')||'#e9edf3 0% 100%'});mask:radial-gradient(transparent 57%,black 58%);`;canvas.replaceWith(ring);return;
 }
 charts.push(new Chart(canvas,{type:'doughnut',data:{labels:items.map(x=>x.label),datasets:[{data:items.map(x=>x.value),backgroundColor:palette,borderWidth:0,hoverOffset:3,spacing:3,borderRadius:3}]},options:{responsive:true,maintainAspectRatio:false,cutout:'78%',plugins:{legend:{display:false},tooltip:{callbacks:{label:ctx=>`${ctx.label}: ${ctx.raw}`}}}}}));
}
function drawPopularGenres(items) {
 const canvas = document.getElementById('popular-genres-chart');
 if (!window.Chart) { fallbackBars(canvas,items);return; }
 const options = chartOptions();options.indexAxis='y';options.scales.x.ticks.precision=0;options.scales.y.grid.display=false;
 charts.push(new Chart(canvas,{type:'bar',data:{labels:items.map(x=>x.label),datasets:[{label:'Выдач',data:items.map(x=>x.value),backgroundColor:'#b78060',borderRadius:4,maxBarThickness:17}]},options}));
}
function fallbackBars(canvas,items) {
 const max = Math.max(...items.map(x=>x.value),1);
 const node = document.createElement('div'); node.innerHTML=`<div class="fallback-bars">${items.map(x=>`<div class="fallback-bar" style="height:${Math.max(1,x.value/max*85)}%"><b>${number(x.value)}</b><span>${esc(x.label)}</span></div>`).join('')}</div>`;
 canvas.replaceWith(node);
}
function optionList(values, current, label) { return `<option value="">${label}</option>${values.map(v=>`<option value="${esc(v)}" ${v===current?'selected':''}>${esc(v)}</option>`).join('')}`; }
async function books(id) {
 const [data,genres] = await Promise.all([api(`/books?${query({...filters,page:pageNumber,limit:10})}`),api('/options/genres')]);
 if (id!==renderID) return;
 records = new Map(data.items.map(b=>[b.id,b]));
 content.innerHTML = heading('Ваша следующая история','Знакомые авторы, неожиданные открытия и книги, к которым хочется возвращаться.',addButton('add-book','Добавить книгу'),'ШКОЛЬНАЯ БИБЛИОТЕКА')+
 `<section class="panel catalog-panel"><div class="catalog-section-heading"><div><h2>На книжной полке</h2><p>${number(data.total)} наименований в каталоге</p></div><div class="catalog-view" role="group" aria-label="Вид каталога"><button type="button" class="icon-button" data-catalog-view="shelf" aria-pressed="${catalogView==='shelf'}" aria-label="Книжная полка">${icon('layout-grid')}</button><button type="button" class="icon-button" data-catalog-view="table" aria-pressed="${catalogView==='table'}" aria-label="Таблица">${icon('list')}</button></div></div><div class="toolbar"><label class="search-input">${icon('search')}<input data-filter="search" value="${esc(filters.search)}" placeholder="Поиск по названию…" aria-label="Поиск по названию"></label><label class="search-input author-search">${icon('pen-line')}<input data-filter="author" value="${esc(filters.author)}" placeholder="Автор" aria-label="Поиск по автору"></label><select data-filter="genre" aria-label="Жанр">${optionList(genres,filters.genre,'Все жанры')}</select><select data-filter="availability" aria-label="Доступность">${optionList(['available','unavailable'],filters.availability,'Любая доступность').replace('>available<','>Доступны<').replace('>unavailable<','>Нет в наличии<')}</select><select data-filter="sort_order" aria-label="Сортировка">${[['title:asc','Название: А–Я'],['title:desc','Название: Я–А'],['author:asc','Автор: А–Я'],['author:desc','Автор: Я–А'],['year:desc','Сначала новые'],['year:asc','Сначала старые']].map(([v,l])=>`<option value="${v}" ${v===`${filters.sort||'title'}:${filters.order||'asc'}`?'selected':''}>${l}</option>`).join('')}</select></div>${data.items.length?(catalogView==='shelf'?bookShelf(data.items):`<div class="table-wrap"><table><thead><tr><th>Название и автор</th><th>Жанр</th><th>Год</th><th>Экземпляры</th><th>Доступность</th><th>Действия</th></tr></thead><tbody>${data.items.map(b=>`<tr><td><button class="row-title book-cell" data-action="view-book" data-id="${b.id}">${bookCover(b.title,b.id)}<span><strong>${esc(b.title)}</strong><small>${esc(b.author)}</small></span></button></td><td><span class="badge neutral">${esc(b.genre)}</span></td><td class="text-muted">${b.year}</td><td><strong>${b.available_copies}</strong><span class="text-muted"> / ${b.total_copies}</span></td><td>${b.available_copies?'<span class="badge success"><span></span>Доступна</span>':'<span class="badge danger"><span></span>Нет в наличии</span>'}</td><td><div class="actions"><button class="icon-button" data-action="issue-book" data-id="${b.id}" ${b.available_copies?'':'disabled'} title="Выдать книгу" aria-label="Выдать книгу">${icon('arrow-up-right')}</button><button class="icon-button" data-action="edit-book" data-id="${b.id}" title="Редактировать" aria-label="Редактировать книгу">${icon('pencil')}</button><button class="icon-button" data-action="delete-book" data-id="${b.id}" title="Удалить" aria-label="Удалить книгу">${icon('trash-2')}</button></div></td></tr>`).join('')}</tbody></table></div>`):empty('Книги не найдены',Object.values(filters).some(Boolean)?'Попробуйте изменить поисковый запрос или фильтры.':'Добавьте книгу, чтобы пополнить библиотеку.')}${pagination(data)}</section>`;
 icons();
}
async function readers(id) {
 const [data,classes] = await Promise.all([api(`/readers?${query({...filters,page:pageNumber,limit:10})}`),api('/options/classes')]);
 if (id!==renderID) return;
 records = new Map(data.items.map(b=>[b.id,b]));
 content.innerHTML = heading('Читатели','Знакомые лица и новые любители книг.',addButton('add-reader','Добавить читателя','user-plus'),'СООБЩЕСТВО БИБЛИОТЕКИ')+
 `<section class="panel"><div class="toolbar"><label class="search-input">${icon('search')}<input data-filter="search" value="${esc(filters.search)}" placeholder="Поиск по ФИО…" aria-label="Поиск по ФИО"></label><select data-filter="class" aria-label="Класс">${optionList(classes,filters.class,'Все классы')}</select><span class="filter-count">Читателей <b>${number(data.total)}</b></span></div>${data.items.length?`<div class="table-wrap"><table><thead><tr><th>Читатель</th><th>Класс</th><th>Дата регистрации</th><th>Книги на руках</th><th>Действия</th></tr></thead><tbody>${data.items.map(r=>`<tr><td><button class="row-title reader-cell" data-action="view-reader" data-id="${r.id}"><span class="reader-avatar">${esc(initials(r.name))}</span><strong>${esc(r.name)}</strong></button></td><td><span class="badge neutral">${esc(r.class)}</span></td><td class="text-muted">${date(r.created_at)}</td><td><button class="text-link row-title" data-action="view-reader" data-id="${r.id}">Посмотреть ${icon('arrow-up-right')}</button></td><td><div class="actions"><button class="icon-button" data-action="issue-reader" data-id="${r.id}" title="Выдать книгу" aria-label="Выдать книгу читателю">${icon('book-plus')}</button><button class="icon-button" data-action="edit-reader" data-id="${r.id}" title="Редактировать" aria-label="Редактировать читателя">${icon('pencil')}</button><button class="icon-button" data-action="delete-reader" data-id="${r.id}" title="Удалить" aria-label="Удалить читателя">${icon('trash-2')}</button></div></td></tr>`).join('')}</tbody></table></div>`:empty('Читатели не найдены','Добавьте читателя или измените условия поиска.')}${pagination(data)}</section>`;
 icons();
}
async function loans(id) {
 const data = await api(`/loans?${query({...filters,status:loanTab,page:pageNumber,limit:10})}`);
 if (id!==renderID) return;
 records = new Map(data.items.map(b=>[b.id,b]));
 content.innerHTML = heading('Выдачи и возвраты','Пусть каждая история вовремя возвращается домой.',addButton('issue-loan','Выдать книгу','arrow-up-right'),'ДВИЖЕНИЕ КНИГ')+
 `<section class="panel"><div class="tabs" role="tablist" aria-label="Тип выдач">${[['active','book-open','Активные выдачи'],['overdue','clock-3','Просроченные'],['history','history','История выдач']].map(([s,i,l])=>`<button class="tab ${s===loanTab?'active':''}" data-tab="${s}" role="tab" aria-selected="${s===loanTab}">${icon(i)}${l}</button>`).join('')}</div><div class="toolbar"><label class="search-input">${icon('search')}<input data-filter="search" value="${esc(filters.search)}" placeholder="Поиск по книге или читателю…" aria-label="Поиск выдач"></label><span class="filter-count">Найдено <b>${number(data.total)}</b></span></div>${loanTable(data.items)}${pagination(data)}</section>`;
 icons();
}
async function render({quiet = false, focus = null} = {}) {
 if (!currentUser) return;
 const id = ++renderID;
 if (!quiet) { charts.forEach(c=>c.destroy()); charts=[];content.innerHTML=skeleton(); }
 try {
  await ({dashboard:()=>dashboard(id),stats:()=>dashboard(id,true),books:()=>books(id),readers:()=>readers(id),loans:()=>loans(id),accounts:()=>accounts(id)})[page]();
  if(id===renderID) applyPermissions();
  if(focus && id===renderID) { const input=content.querySelector(`[data-filter="${focus.key}"]`);if(input){input.focus();if(input.setSelectionRange)input.setSelectionRange(focus.position,focus.position);} }
 } catch (error) {
  if (id!==renderID) return;
  content.innerHTML=heading(pageNames[page],'Управление школьной библиотекой')+`<div class="panel error-state">${icon('wifi-off')}<h2>Не удалось загрузить данные</h2><p>${esc(error.message)}</p><button class="button primary" data-action="retry">${icon('refresh-cw')}Попробовать снова</button></div>`;icons();
 }
}
function navigate() {
 if (!currentUser) return;
 const next = location.hash.slice(1).split('?')[0] || 'dashboard';
 clearTimeout(searchTimer);page = pageNames[next]?next:'dashboard';if(isReader()&&!['books','loans'].includes(page)){page='books';history.replaceState(null,'','#books');}pageNumber=1;filters={};closeSidebar();
 document.querySelectorAll('nav [data-page]').forEach(a=>{a.classList.toggle('active',a.dataset.page===page);if(a.dataset.page===page)a.setAttribute('aria-current','page');else a.removeAttribute('aria-current');});
 document.getElementById('crumb').textContent = page==='dashboard'?'Обзор':pageNames[page];
 document.title=`${pageNames[page]} · LibraryHub`;
 render();
}
window.addEventListener('hashchange',navigate);
let searchTimer;
content.addEventListener('input',event=>{
 const key=event.target.dataset.filter;if(!key||event.target.tagName==='SELECT')return;
 const value=event.target.value, position=event.target.selectionStart;
 clearTimeout(searchTimer);++renderID;filters[key]=value;pageNumber=1;
 const expectedPage=page;
 searchTimer=setTimeout(()=>{if(page===expectedPage)render({quiet:true,focus:{key,position}});},350);
});
content.addEventListener('change',event=>{
 const key=event.target.dataset.filter;if(!key||event.target.tagName!=='SELECT')return;
 if(key==='sort_order'){[filters.sort,filters.order]=event.target.value.split(':');}else{filters[key]=event.target.value;}
 pageNumber=1;render({quiet:true});
});
content.addEventListener('click',event=>{
 const view=event.target.closest('[data-catalog-view]');if(view){catalogView=view.dataset.catalogView;try{localStorage.setItem('libraryhub-catalog-view',catalogView);}catch{}render({quiet:true});return;}
 const pager=event.target.closest('[data-pagination]');if(pager){pageNumber=Number(pager.dataset.pagination);render({quiet:true});return;}
 const tab=event.target.closest('[data-tab]');if(tab){loanTab=tab.dataset.tab;pageNumber=1;render({quiet:true});return;}
 const button=event.target.closest('[data-action]');if(!button)return;
 const id=Number(button.dataset.id);
 handleAction(button.dataset.action,id).catch(e=>toast(e.message,true));
});
async function handleAction(action,id) {
 if(isReader()&&!['retry','view-book'].includes(action))return;
 switch(action){
  case 'retry':return render();
  case 'change-role':return changeRole(records.get(id));
  case 'add-book':return openBook();
  case 'add-librarian':return openAccount();
  case 'edit-book':return openBook(await api(`/books/${id}`));
  case 'view-book':return viewBook(id);
  case 'delete-book':return confirmDelete('books',id,records.get(id)?.title);
  case 'add-reader':return openReader();
  case 'edit-reader':return openReader(await api(`/readers/${id}`));
  case 'view-reader':return viewReader(id);
  case 'delete-reader':return confirmDelete('readers',id,records.get(id)?.name);
  case 'issue-loan':return openLoan();
  case 'issue-book':return openLoan(id);
  case 'issue-reader':return openLoan(null,id);
  case 'return-loan':return openReturn(id,records.get(id));
 }
}
function modal(title, subtitle, body, footer = '') {
 modalID++;
 modalContent.innerHTML=`<div class="modal-head"><div><h2 id="modal-title">${title}</h2>${subtitle?`<p>${subtitle}</p>`:''}</div><button class="icon-button" data-close aria-label="Закрыть">${icon('x')}</button></div>${body}${footer}`;
 if(!dialog.open) dialog.showModal();
 icons();return modalID;
}
dialog.addEventListener('click',event=>{if(event.target.closest('[data-close]'))dialog.close();});
dialog.addEventListener('close',()=>{modalID++;});
const field=(name,label,value='',type='text',attributes='',full=false)=>`<label class="field ${full?'full':''}">${label}<input name="${name}" type="${type}" value="${esc(value)}" ${attributes}></label>`;
function formBody(fields,note='') { return `<form id="editor"><div class="modal-body"><div class="form-error" id="form-error" role="alert"></div>${note?`<div class="modal-note">${note}</div>`:''}<div class="form-grid">${fields}</div></div><div class="modal-footer"><button class="button" type="button" data-close>Отмена</button><button class="button primary" type="submit">${icon('check')}Сохранить</button></div></form>`; }
function submitForm(fn,message) {
 const form=document.getElementById('editor');const token=modalID;
 form.addEventListener('submit',async event=>{
  event.preventDefault();const submit=form.querySelector('[type=submit]');if(submit.disabled)return;
  const errorBox=form.querySelector('#form-error');errorBox.textContent='';submit.disabled=true;submit.setAttribute('aria-busy','true');
  try{await fn(Object.fromEntries(new FormData(form)));if(token===modalID)dialog.close();toast(message);render();}
  catch(error){if(token===modalID)errorBox.textContent=error.message;else toast(error.message,true);}
  finally{submit.disabled=false;submit.removeAttribute('aria-busy');}
 });
}
function openBook(b={}) {
 const fields=field('title','Название книги *',b.title,'text','required maxlength="250"',true)+field('author','Автор *',b.author,'text','required maxlength="200"',true)+
 field('genre','Жанр *',b.genre,'text','required maxlength="100" list="genre-suggestions"')+field('year','Год издания *',b.year||new Date().getFullYear(),'number',`required min="1" max="${new Date().getFullYear()+1}"`)+
 field('total_copies','Всего экземпляров *',b.total_copies??1,'number','required min="0" max="100000"')+field('isbn','ISBN',b.isbn,'text','maxlength="32"')+
 `<label class="field full">Описание<textarea name="description" maxlength="5000" placeholder="О чём эта книга?">${esc(b.description)}</textarea></label>`;
 const token=modal(b.id?'Редактировать книгу':'Новая книга','Заполните информацию для каталога',formBody(fields,b.id?`Сейчас на руках: ${b.total_copies-b.available_copies}. Доступное количество пересчитывается автоматически.`:''));
 const list=document.createElement('datalist');list.id='genre-suggestions';document.getElementById('editor').append(list);
 api('/options/genres').then(items=>{if(token===modalID)list.innerHTML=items.map(x=>`<option value="${esc(x)}"></option>`).join('');}).catch(()=>{});
 submitForm(data=>api(b.id?`/books/${b.id}`:'/books',{method:b.id?'PUT':'POST',body:{...data,year:Number(data.year),total_copies:Number(data.total_copies)}}),b.id?'Книга обновлена':'Книга добавлена в каталог');
}
function openReader(r={}) {
 modal(r.id?'Редактировать читателя':'Новый читатель','Познакомимся с будущим любителем книг',formBody(field('name','ФИО ученика *',r.name,'text','required minlength="2" maxlength="200"',true)+field('class','Класс *',r.class,'text','required maxlength="20" placeholder="Например, 9А"',true)));
 submitForm(data=>api(r.id?`/readers/${r.id}`:'/readers',{method:r.id?'PUT':'POST',body:data}),r.id?'Данные читателя обновлены':'Читатель добавлен');
}
async function viewBook(id) {
 const b=await api(`/books/${id}`);
 modal('Карточка книги','',`<div class="modal-body"><div class="detail-book">${bookCover(b.title,b.id,false,b.author)}<div><h3>${esc(b.title)}</h3><p>${esc(b.author)}</p><span class="badge ${b.available_copies?'success':'danger'}">${b.available_copies?'Доступна для выдачи':'Нет свободных экземпляров'}</span></div></div><dl class="detail-meta"><div><dt>Жанр</dt><dd>${esc(b.genre)}</dd></div><div><dt>Год издания</dt><dd>${b.year}</dd></div><div><dt>Доступно / всего</dt><dd>${b.available_copies} / ${b.total_copies}</dd></div><div><dt>ISBN</dt><dd>${esc(b.isbn)||'Не указан'}</dd></div><div><dt>Добавлена</dt><dd>${date(b.created_at)}</dd></div></dl><p class="description">${esc(b.description)||'Описание пока не добавлено.'}</p></div>`,`<div class="modal-footer"><button class="button" id="detail-edit">${icon('pencil')}Редактировать</button><button class="button primary" id="detail-issue" ${b.available_copies?'':'disabled'}>${icon('arrow-up-right')}Выдать книгу</button></div>`);
 if(isReader()){modalContent.querySelector('.modal-footer')?.remove();return;}
 document.getElementById('detail-edit').onclick=()=>openBook(b);
 document.getElementById('detail-issue').onclick=()=>openLoan(id);
}
async function viewReader(id) {
 const [r,data]=await Promise.all([api(`/readers/${id}`),api(`/loans?reader_id=${id}&status=active&limit=10`)]);
 const token=modal(esc(r.name),`${esc(r.class)} класс · В библиотеке с ${date(r.created_at)}`,`<div class="modal-body"><h2>Книги на руках <span class="text-muted">(${data.total})</span></h2><div id="reader-loans">${readerLoans(data)}</div><div id="reader-loans-pager"></div></div>`,`<div class="modal-footer"><button class="button" id="detail-edit">${icon('pencil')}Редактировать</button><button class="button primary" id="detail-issue">${icon('book-plus')}Выдать книгу</button></div>`);
 document.getElementById('detail-edit').onclick=()=>openReader(r);
 document.getElementById('detail-issue').onclick=()=>openLoan(null,id);
 const accountButton=document.createElement('button');accountButton.className='button';accountButton.textContent='Создать аккаунт';accountButton.onclick=()=>openAccount(r);modalContent.querySelector('.modal-footer').prepend(accountButton);
 const wire=(result)=>{
  const area=document.getElementById('reader-loans');area.innerHTML=readerLoans(result);
  document.getElementById('reader-loans-pager').innerHTML=result.total>result.limit?pagination(result):'';
  area.querySelectorAll('[data-reader-return]').forEach(button=>button.onclick=()=>openReturn(Number(button.dataset.readerReturn),result.items.find(l=>l.id===Number(button.dataset.readerReturn))));
  document.getElementById('reader-loans-pager').querySelectorAll('[data-pagination]').forEach(button=>button.onclick=async()=>{button.disabled=true;try{const next=await api(`/loans?reader_id=${id}&status=active&limit=10&page=${button.dataset.pagination}`);if(modalID===token)wire(next);}catch(e){toast(e.message,true);button.disabled=false;}});
  icons();
 };wire(data);
}
function readerLoans(data) {
 return data.items.length?data.items.map(l=>`<div class="reader-loan"><div><strong>${esc(l.book_title)}</strong><small class="${l.overdue?'text-red':''}">До ${date(l.due_at)}${l.overdue?' · Просрочена':''}</small></div><button class="button small" data-reader-return="${l.id}">Вернуть</button></div>`).join(''):empty('Все книги возвращены','У читателя нет активных выдач.');
}
function confirmDelete(kind,id,name) {
 modal('Удалить запись?','Это действие нельзя отменить.',`<form id="editor"><div class="modal-body"><div class="form-error" id="form-error" role="alert"></div><div class="confirm-icon">${icon('trash-2')}</div><p>Вы собираетесь удалить <strong>${esc(name)||'эту запись'}</strong>. История завершённых выдач сохранится.</p><p>Запись с книгами на руках удалить нельзя.</p></div><div class="modal-footer"><button class="button" type="button" data-close>Отмена</button><button class="button danger-button" type="submit">Удалить</button></div></form>`);
 submitForm(()=>api(`/${kind}/${id}`,{method:'DELETE'}),'Запись удалена');
}
function openReturn(id,loan) {
 modal('Возврат книги','Зафиксируйте фактическую дату возврата',formBody(field('returned_at','Дата возврата *',localDay(),'date',`required max="${localDay()}" ${loan?`min="${loan.issued_at.slice(0,10)}"`:''}`,true),loan?`${esc(loan.book_title)} · ${esc(loan.reader_name)}`:''));
 document.querySelector('#editor [type=submit]').innerHTML=`${icon('corner-down-left')}Принять книгу`;icons();
 submitForm(data=>api(`/loans/${id}/return`,{method:'POST',body:data}),'Книга возвращена в библиотеку');
}
function openLoan(bookID=null,readerID=null) {
 const token=modal('Выдать книгу','Выберите книгу, читателя и срок возврата',formBody(
 `<div class="field full"><label for="book-search">Книга *</label><div class="search-input">${icon('search')}<input id="book-search" placeholder="Найти книгу по названию…" autocomplete="off"></div><select name="book_id" required aria-label="Выберите книгу"><option value="">Загрузка книг…</option></select><small id="books-picker-note"></small><button class="button small ghost" id="books-more" type="button" hidden>Загрузить ещё</button></div>`+
 `<div class="field full"><label for="reader-search">Читатель *</label><div class="search-input">${icon('search')}<input id="reader-search" placeholder="Найти читателя по ФИО…" autocomplete="off"></div><select name="reader_id" required aria-label="Выберите читателя"><option value="">Загрузка читателей…</option></select><small id="readers-picker-note"></small><button class="button small ghost" id="readers-more" type="button" hidden>Загрузить ещё</button></div>`+
 field('issued_at','Дата выдачи *',localDay(),'date',`required max="${localDay()}"`)+field('due_at','Вернуть до *',localDay(14),'date',`required min="${localDay()}"`)
 ));
 const form=document.getElementById('editor');const submit=form.querySelector('[type=submit]');submit.innerHTML=`${icon('arrow-up-right')}Выдать книгу`;icons();
 setupPicker('books',bookID,token);
 setupPicker('readers',readerID,token);
 const issueInput=form.querySelector('[name=issued_at]');issueInput.onchange=()=>form.querySelector('[name=due_at]').min=issueInput.value;
 submitForm(data=>api('/loans',{method:'POST',body:{book_id:Number(data.book_id),reader_id:Number(data.reader_id),issued_at:data.issued_at,due_at:data.due_at}}),'Книга выдана читателю');
}
function setupPicker(kind,selectedID,token) {
 const isBooks=kind==='books';
 const form=document.getElementById('editor'),select=form.querySelector(`[name=${isBooks?'book_id':'reader_id'}]`),search=document.getElementById(isBooks?'book-search':'reader-search'),more=document.getElementById(`${kind}-more`),note=document.getElementById(`${kind}-picker-note`);
 let request=0,loaded=0,pickerPage=0,term='',timer;
 let selection=null;const objects=new Map();
 const label=x=>isBooks?`${x.title} — ${x.author} (${x.available_copies} доступно)`:`${x.name} · ${x.class}`;
 async function load(append=false) {
  append=append&&pickerPage>0;const nextPage=append?pickerPage+1:1;
  const current=++request;more.disabled=true;select.disabled=true;note.textContent='Загрузка…';
  try {
   const [data,preset]=await Promise.all([api(`/${kind}?${query({search:term,availability:isBooks?'available':'',limit:30,page:nextPage})}`),selectedID&&!selection?api(`/${kind}/${selectedID}`):Promise.resolve(null)]);
   if(token!==modalID||current!==request)return;
   if(preset){selection=preset;objects.set(preset.id,preset);selectedID=null;}
   if(!append){select.innerHTML=`<option value="">${isBooks?'Выберите книгу':'Выберите читателя'}</option>`;loaded=0;}
   const ids=new Set(Array.from(select.options).map(x=>x.value));
   if(selection&&!ids.has(String(selection.id))){const option=new Option(label(selection),selection.id);if(isBooks&&selection.available_copies<1)option.disabled=true;select.add(option);ids.add(String(selection.id));}
   data.items.forEach(x=>{objects.set(x.id,x);if(!ids.has(String(x.id)))select.add(new Option(label(x),x.id));});
   pickerPage=nextPage;more.textContent='Загрузить ещё';
   loaded+=data.items.length;more.hidden=loaded>=data.total;note.textContent=data.total?`Найдено: ${data.total}. ${isBooks?'Показаны только доступные книги.':'Для поиска введите ФИО.'}`:`${isBooks?'Доступных книг':'Читателей'} не найдено.`;
   if(selection)select.value=String(selection.id);
  }catch(error){if(token===modalID&&current===request){note.textContent=error.message;more.hidden=false;more.textContent='Повторить загрузку';}}
  finally{if(token===modalID&&current===request){more.disabled=false;select.disabled=false;}}
 }
 search.oninput=()=>{clearTimeout(timer);++request;term=search.value;pickerPage=0;timer=setTimeout(()=>load(),300);};
 select.onchange=()=>{selection=objects.get(Number(select.value))||null;};
 more.onclick=()=>load(true);load();
}
bootAuth();

function applyPermissions() {
 if(!isReader())return;
 document.querySelectorAll('nav [data-page]').forEach(a=>a.hidden=!['books','loans'].includes(a.dataset.page));
 content.querySelectorAll('[data-action]').forEach(b=>{if(b.dataset.action==='view-reader'){const label=document.createElement('span');label.className='reader-cell';label.innerHTML=b.innerHTML;b.replaceWith(label);}else if(!['view-book','retry'].includes(b.dataset.action))b.remove();});
 content.querySelectorAll('.heading-actions,.actions').forEach(e=>e.remove());
 // Remove action/return columns rather than leaving an empty administrative table.
 content.querySelectorAll('table').forEach(table=>{
  const heads=[...table.querySelectorAll('thead th')];
  const index=heads.findIndex(h=>['Действия','Возврат'].includes(h.textContent.trim()));
  if(index>=0)table.querySelectorAll('tr').forEach(row=>row.children[index]?.remove());
 });
 if(page==='loans') {document.getElementById('crumb').textContent='Мои книги';document.title='Мои книги · LibraryHub';content.querySelector('h1').textContent='Мои книги';content.querySelector('.page-heading p').textContent='Ваши выдачи, сроки возврата и история чтения.';}
}
function showLogin(message='') {
 closeAccountMenu();currentUser=null;++renderID;charts.forEach(c=>c.destroy());charts=[];dialog.close();
 document.body.classList.add('signed-out');content.innerHTML='';
 let screen=document.getElementById('login-screen');if(!screen){screen=document.createElement('section');screen.id='login-screen';document.body.append(screen);}
 screen.innerHTML=`<div class="login-card"><div class="eyebrow">LIBRARYHUB</div><h1>С возвращением</h1><p>Войдите, чтобы продолжить свою историю.</p><form id="login-form"><div class="form-error" role="alert">${esc(message)}</div><label class="field">Логин<input name="login" autocomplete="username" required maxlength="64" autofocus></label><label class="field">Пароль<input name="password" type="password" autocomplete="current-password" required maxlength="72"></label><button class="button primary" type="submit">Войти в библиотеку</button></form><p class="login-hint">Нет аккаунта? <button type="button" class="button ghost" id="register-link">Зарегистрироваться</button></p></div>`;
 screen.querySelector('#register-link').onclick=showRegistration;
 screen.querySelector('form').onsubmit=async event=>{event.preventDefault();const form=event.currentTarget,button=form.querySelector('button');button.disabled=true;try{currentUser=await api('/auth/login',{method:'POST',body:Object.fromEntries(new FormData(form))});finishLogin();}catch(e){form.querySelector('.form-error').textContent=e.message;}finally{button.disabled=false;}};
}
function finishLogin() {
 document.getElementById('login-screen')?.remove();document.body.classList.remove('signed-out');
 if(!document.querySelector('nav [data-page=accounts]')){const link=document.createElement('a');link.href='#accounts';link.dataset.page='accounts';link.innerHTML=`${icon('shield-check')}Пользователи`;document.querySelector('nav').append(link);}
 document.querySelectorAll('nav [data-page]').forEach(a=>a.hidden=isReader()&&!['books','loans'].includes(a.dataset.page));
 const loansLink=document.querySelector('nav [data-page=loans]');loansLink.innerHTML=`${icon('arrow-left-right')}${isReader()?'Мои книги':'Выдачи и возвраты'}<span id="loan-count" class="nav-count" hidden></span>`;
 document.querySelector('.topbar .avatar').textContent=currentUser.login[0].toUpperCase();
 const menu=document.getElementById('account-menu');
 menu.innerHTML=`<div class="account-menu-profile" role="presentation"><span class="avatar">${esc(currentUser.login[0].toUpperCase())}</span><div><strong>${esc(currentUser.login)}</strong><small>${isReader()?'Читатель':'Библиотекарь'}</small></div></div><div class="account-menu-actions" role="presentation">${isReader()?'':`<button type="button" role="menuitem" id="new-librarian">${icon('user-plus')}Новый библиотекарь</button>`}<button type="button" role="menuitem" id="logout">${icon('log-out')}Выйти / сменить аккаунт</button></div>`;
 document.getElementById('new-librarian')?.addEventListener('click',()=>{closeAccountMenu();openAccount();});
 document.getElementById('logout').onclick=async()=>{const button=document.getElementById('logout');button.disabled=true;try{await api('/auth/logout',{method:'POST',body:{}});closeAccountMenu();showLogin();}catch(e){toast(e.message,true);}finally{button.disabled=false;}};
 icons();
 navigate();
}
function openAccount(reader=null) {
 modal('Создать аккаунт',reader?`${esc(reader.name)} · Читатель`:'Роль: Библиотекарь',formBody(field('login','Логин *','','text','required minlength="3" maxlength="64" pattern="[a-zA-Z0-9_.-]+" autocomplete="off"',true)+field('password','Пароль *','','password','required minlength="10" maxlength="72" autocomplete="new-password"',true),'Передайте логин и пароль владельцу аккаунта лично.'));
 submitForm(data=>api('/accounts',{method:'POST',body:{...data,role:reader?'reader':'librarian',reader_id:reader?.id??null}}),'Аккаунт создан');
}
async function bootAuth() {
 document.body.classList.add('signed-out');
 try{currentUser=await api('/auth/me');finishLogin();}catch(e){showLogin(e.status===401?'':e.message);}
}
window.addEventListener('libraryhub-session-expired',()=>{if(currentUser)showLogin('Сессия истекла. Войдите снова.');});


function showRegistration() {
 const screen=document.getElementById('login-screen');
 screen.innerHTML=`<div class="login-card"><div class="eyebrow">LIBRARYHUB</div><h1>Стать читателем</h1><p>Создайте аккаунт для каталога и своих выдач.</p><form id="registration-form"><div class="form-error" role="alert"></div>${field('name','ФИО *','','text','required minlength="2" maxlength="200" autocomplete="name"')}${field('class','Класс *','','text','required maxlength="20" placeholder="Например, 9А"')}${field('login','Логин *','','text','required minlength="3" maxlength="64" pattern="[a-zA-Z0-9_.-]+" autocomplete="username"')}<small>Латинские буквы, цифры, точка, дефис или подчёркивание.</small>${field('password','Пароль *','','password','required minlength="10" maxlength="72" autocomplete="new-password"')}${field('confirmation','Повторите пароль *','','password','required autocomplete="new-password"')}<button class="button primary" type="submit">Зарегистрироваться</button></form><p class="login-hint">Уже есть аккаунт? <button class="button ghost" id="back-login" type="button">Войти</button></p></div>`;
 screen.querySelector('#back-login').onclick=()=>showLogin();
 screen.querySelector('form').onsubmit=async event=>{
  event.preventDefault();const form=event.currentTarget,button=form.querySelector('[type=submit]'),error=form.querySelector('.form-error'),data=Object.fromEntries(new FormData(form));error.textContent='';
  if(data.password!==data.confirmation){error.textContent='Пароли не совпадают';return;}delete data.confirmation;button.disabled=true;
  try{await api('/auth/register',{method:'POST',body:data});showLogin();document.querySelector('#login-form [name=login]').value=data.login;toast('Аккаунт создан. Теперь войдите с вашим паролем.');}catch(e){error.textContent=e.message;}finally{button.disabled=false;}
 };
}
async function accounts(id) {
 const data=await api(`/accounts?${query({search:filters.search,page:pageNumber})}`);if(id!==renderID)return;
 records=new Map(data.items.map(u=>[u.id,u]));
 content.innerHTML=heading('Пользователи','Аккаунты и права доступа к библиотеке.',addButton('add-librarian','Новый библиотекарь'))+`<section class="panel"><div class="toolbar"><label class="search-input">${icon('search')}<input data-filter="search" value="${esc(filters.search)}" placeholder="Поиск по логину или ФИО…" aria-label="Поиск пользователей"></label></div>${data.items.length?`<div class="table-wrap"><table><thead><tr><th>Пользователь</th><th>Читатель</th><th>Роль</th><th>Действие</th></tr></thead><tbody>${data.items.map(u=>`<tr><td>${esc(u.login)}${u.id===currentUser.id?' (вы)':''}</td><td>${esc(u.name)||'—'}${u.class?` · ${esc(u.class)}`:''}${u.reader_id&&!u.reader_active?' · Профиль удалён':''}</td><td><span class="badge ${u.role==='librarian'?'success':'neutral'}">${u.role==='librarian'?'Библиотекарь':'Читатель'}</span></td><td>${u.reader_active?`<button class="button small" data-action="change-role" data-id="${u.id}">${u.role==='reader'?'Назначить библиотекарем':'Снять роль библиотекаря'}</button>`:'<span class="text-muted">Нет активного профиля читателя</span>'}</td></tr>`).join('')}</tbody></table></div>`:empty('Пользователи не найдены','Измените поисковый запрос.')}${pagination(data)}</section>`;icons();
}
function changeRole(user) {
 if(!user)return;const role=user.role==='reader'?'librarian':'reader';
 modal(role==='librarian'?'Назначить библиотекарем?':'Снять роль библиотекаря?',esc(user.login),formBody('',role==='librarian'?'Пользователь получит полный доступ к книгам, читателям, выдачам и управлению ролями. После изменения потребуется повторный вход.':'Пользователь сохранит профиль и историю чтения, но потеряет административный доступ. Все его сессии будут завершены.'));
 submitForm(async()=>{await api(`/accounts/${user.id}/role`,{method:'PATCH',body:{role}});if(user.id===currentUser.id)showLogin('Ваша роль изменена. Войдите снова.');},'Роль пользователя обновлена');
}

function closeAccountMenu(restoreFocus=false) {
 const trigger=document.getElementById('account-trigger');
 document.getElementById('account-menu').hidden=true;trigger.setAttribute('aria-expanded','false');
 if(restoreFocus)trigger.focus();
}
function openAccountMenu() {
 if(!currentUser)return;
 document.getElementById('account-menu').hidden=false;
 document.getElementById('account-trigger').setAttribute('aria-expanded','true');
 document.querySelector('#account-menu [role=menuitem]')?.focus();
}
document.getElementById('account-trigger').onclick=()=>{
 if(document.getElementById('account-menu').hidden)openAccountMenu();else closeAccountMenu(true);
};
document.getElementById('account-trigger').addEventListener('keydown',event=>{
 if(event.key==='ArrowDown'){event.preventDefault();openAccountMenu();}
});
document.addEventListener('pointerdown',event=>{if(!event.target.closest('#account-dropdown'))closeAccountMenu();});
document.addEventListener('focusin',event=>{if(!event.target.closest('#account-dropdown'))closeAccountMenu();});
document.getElementById('account-dropdown').addEventListener('keydown',event=>{
 if(event.key==='Escape'){event.preventDefault();closeAccountMenu(true);return;}
 if(document.getElementById('account-menu').hidden)return;
 const buttons=[...document.querySelectorAll('#account-menu [role=menuitem]')];const i=buttons.indexOf(document.activeElement);
 let next;if(event.key==='ArrowDown')next=(i+1)%buttons.length;else if(event.key==='ArrowUp')next=(i-1+buttons.length)%buttons.length;else if(event.key==='Home')next=0;else if(event.key==='End')next=buttons.length-1;
 if(next!==undefined){event.preventDefault();buttons[next]?.focus();}
});

function bookShelf(items) {
 return `<div class="book-shelf">${items.map(b=>`<article class="shelf-card"><button class="shelf-cover-button" data-action="view-book" data-id="${b.id}" aria-label="Открыть книгу ${esc(b.title)}"><span class="shelf-genre">${esc(b.genre)}</span>${bookCover(b.title,b.id,false,b.author)}</button><div class="shelf-info"><button class="row-title" data-action="view-book" data-id="${b.id}"><h3>${esc(b.title)}</h3></button><p>${esc(b.author)}</p><div class="shelf-status"><span class="availability ${b.available_copies?'':'unavailable'}">${b.available_copies?'Есть в библиотеке':'Все экземпляры на руках'}</span><small>${b.available_copies} / ${b.total_copies}</small></div><div class="actions shelf-actions"><button class="button small" data-action="issue-book" data-id="${b.id}" ${b.available_copies?'':'disabled'}>${icon('arrow-up-right')}Выдать</button><button class="icon-button" data-action="edit-book" data-id="${b.id}" aria-label="Редактировать книгу">${icon('pencil')}</button><button class="icon-button" data-action="delete-book" data-id="${b.id}" aria-label="Удалить книгу">${icon('trash-2')}</button></div></div></article>`).join('')}</div>`;
}
