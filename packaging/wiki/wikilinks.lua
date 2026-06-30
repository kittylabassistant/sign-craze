-- wikilinks.lua — pandoc Lua-фильтр для рендеринга GitHub-wiki в статический сайт.
--
-- Переписывает внутренние ссылки вики в относительные .html:
--   [Installation](Installation)      -> Installation.html
--   [Reference](Routing-Reference.md) -> Routing-Reference.html
--   [Home](Home)                      -> index.html
--   [Раздел](Routing#anchor)          -> Routing.html#anchor
--
-- Не трогает: внешние ссылки (http:, mailto:, …), чистые якоря (#…),
-- и любые цели, содержащие '/' (подпути/абсолютные).

function Link(el)
  local t = el.target

  -- scheme (http:, https:, mailto:, ftp:, …) — пропускаем
  if t:match('^%a[%w+.-]*:') then return el end
  -- чистый якорь на текущей странице — пропускаем
  if t:match('^#') then return el end
  -- содержит '/' — это подпуть/абсолют, не wiki-страница
  if t:match('/') then return el end

  -- разбить на путь и якорь
  local path, frag = t:match('^([^#]*)(#?.*)$')

  -- убрать расширение .md, если есть
  path = path:gsub('%.md$', '')

  -- пустой путь (например ссылка вида "#frag" уже отсеяна выше) — пропускаем
  if path == '' then return el end

  -- Home — это корневой index сайта
  if path == 'Home' then path = 'index' end

  el.target = path .. '.html' .. frag
  return el
end
