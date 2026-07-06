# Third-Party Licenses

Sign-Craze использует следующие сторонние компоненты.
Каждый компонент распространяется на условиях своей лицензии,
указанной ниже. Лицензия Sign-Craze (BSD 3-Clause) распространяется
**только** на исходный код Sign-Craze и не затрагивает эти компоненты.

---

## [sing-box](https://sing-box.sagernet.org/)

- **Репозиторий**: <https://github.com/SagerNet/sing-box>
- **Использование**: загружается в рантайме как отдельный исполняемый файл,
  не линкуется со Sign-Craze.
- **Лицензия**: GNU General Public License v3.0 (GPL-3.0)

```plain
Copyright (C) 2022 by nekohasekai <contact-sagernet@sekai.icu>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.

In addition, no derivative work may use the name or imply association
with this application without prior consent.
```

Полный текст лицензии: <https://www.gnu.org/licenses/gpl-3.0.txt>

---

## [Xray-core](https://xtls.github.io/)

- **Репозиторий**: <https://github.com/XTLS/Xray-core>
- **Использование**: загружается в рантайме как отдельный исполняемый файл
  (альтернативное прокси-ядро), не линкуется со Sign-Craze. Для arm64/arm7
  берётся последний релиз upstream; для mips/mipsle используется пин v25.12.8 —
  пересборка исходников Xray-core из
  <https://github.com/kittylabassistant/sign-craze-xray>
  (тег `xray-mips-v25.12.8`), лицензия та же.
- **Лицензия**: Mozilla Public License Version 2.0 (MPL-2.0)

```plain
This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at http://mozilla.org/MPL/2.0/.
```

Полный текст лицензии: <https://www.mozilla.org/MPL/2.0/>

---

## mihomo

- **Репозиторий**: <https://github.com/MetaCubeX/mihomo>
- **Использование**: загружается в рантайме как отдельный исполняемый файл
  (альтернативное прокси-ядро), не линкуется со Sign-Craze.
- **Лицензия**: GNU General Public License v3.0 (GPL-3.0)

Полный текст лицензии: <https://www.gnu.org/licenses/gpl-3.0.txt>

---

## naiveproxy

- **Репозиторий**: <https://github.com/klzgrad/naiveproxy>
- **Использование**: загружается в рантайме как отдельный исполняемый файл
  (supervised peer перед sing-box, SOCKS5 на 127.0.0.1), не линкуется
  со Sign-Craze. Для big-endian mips upstream бинари не публикует.
- **Лицензия**: BSD 3-Clause «New» or «Revised» License (The Chromium Authors)

```plain
Copyright 2015 The Chromium Authors. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google Inc. nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

---

## mieru

- **Репозиторий**: <https://github.com/enfein/mieru>
- **Версия**: v3.32.0 (пин `MIERU_VERSION` в `Makefile`)
- **Использование**: собирается на этапе релиза отдельным `go install`
  (mieru-пакеты не импортируются в Sign-Craze) и опционально включается
  в offline-bundle как отдельный исполняемый файл (supervised peer,
  ADR-0020). В рантайме не скачивается.
- **Лицензия**: GNU General Public License v3.0 (GPL-3.0)

Полный текст лицензии: <https://www.gnu.org/licenses/gpl-3.0.txt>

---

## bol-van/zapret2 (nfqws2)

- **Репозиторий**: <https://github.com/bol-van/zapret2>
- **Использование**: загружается в рантайме как отдельный исполняемый файл
  через nfqws2-keenetic, не линкуется со Sign-Craze.
- **Лицензия**: MIT
- **Расположение лицензии в upstream**: `docs/LICENSE.txt`

```plain
MIT License

Copyright (c) 2016-2025 bol-van

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

---

## nfqws/nfqws2-keenetic

- **Репозиторий**: <https://github.com/nfqws/nfqws2-keenetic>
- **Использование**: предоставляет собранные под архитектуры Keenetic бинари
  nfqws2 и пресеты стратегий DPI-обхода; загружается в рантайме.
- **Лицензия**: MIT

```plain
MIT License

Copyright (c) 2026 nfqws

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

---

## Go standard library and runtime

- **Репозиторий**: <https://github.com/golang/go>
- **Версия**: Go 1.25.9
- **Использование**: стандартная библиотека и рантайм статически
  компилируются в бинарь Sign-Craze.
- **Лицензия**: BSD 3-Clause «New» or «Revised» License

```plain
Copyright 2009 The Go Authors.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google LLC nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

Полный список сторонних компонентов Go-рантайма:
<https://cs.opensource.google/go/go/+/refs/tags/go1.25.9:PATENTS>

---

## golang.org/x/sys

- **Репозиторий**: <https://github.com/golang/sys>
- **Версия**: v0.43.0 (зафиксировано в go.mod)
- **Использование**: системные вызовы Linux (netlink, ioctl, sysctl);
  статически компилируется в бинарь Sign-Craze.
- **Лицензия**: BSD 3-Clause «New» or «Revised» License

```plain
Copyright 2009 The Go Authors.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google LLC nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

---

## golang.org/x/term

- **Репозиторий**: <https://github.com/golang/term>
- **Версия**: v0.42.0 (зафиксировано в go.mod)
- **Использование**: определение TTY и поддержка `--no-color`/`NO_COLOR`
  (`internal/cli/color.go`, `internal/log/log.go`);
  статически компилируется в бинарь Sign-Craze.
- **Лицензия**: BSD 3-Clause «New» or «Revised» License

```plain
Copyright 2009 The Go Authors.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google LLC nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

---

## github.com/ulikunitz/xz

- **Репозиторий**: <https://github.com/ulikunitz/xz>
- **Версия**: v0.5.15 (зафиксировано в go.mod)
- **Использование**: распаковка .tar.xz-архивов при загрузке ядер
  (sing-box, xray, mihomo); статически компилируется в бинарь Sign-Craze.
- **Лицензия**: BSD 3-Clause «New» or «Revised» License

```plain
Copyright (c) 2014-2022, Ulrich Kunitz
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

 * Redistributions of source code must retain the above copyright notice,
   this list of conditions and the following disclaimer.
 * Redistributions in binary form must reproduce the above copyright
   notice, this list of conditions and the following disclaimer in the
   documentation and/or other materials provided with the distribution.
 * The name of the author may not be used to endorse or promote products
   derived from this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

---

## Loyalsoldier/v2ray-rules-dat (geosite.dat / geoip.dat)

- **Репозиторий**: <https://github.com/Loyalsoldier/v2ray-rules-dat>
- **Использование**: файлы данных `geosite.dat`/`geoip.dat` загружаются
  в рантайме для маршрутизации ядра xray (`internal/geo/dat.go`).
- **Лицензия**: GNU General Public License v3.0 (GPL-3.0)
- **Примечание**: `geoip.dat` собирается upstream'ом из базы MaxMind
  GeoLite2 — данные GeoLite2 © MaxMind, Inc.
  (<https://www.maxmind.com>), предоставляются на условиях GeoLite2 EULA.

Полный текст лицензии: <https://www.gnu.org/licenses/gpl-3.0.txt>

---

## SagerNet/sing-geosite и SagerNet/sing-geoip

- **Репозитории**: <https://github.com/SagerNet/sing-geosite>,
  <https://github.com/SagerNet/sing-geoip> (ветка `rule-set`)
- **Использование**: пресеты Routing Editor записывают URL скомпилированных
  rule-set (`.srs`) в `routing.json` (`internal/preset/preset.go`);
  загрузку выполняет сам процесс sing-box при старте.
- **Лицензия**: GNU General Public License v3.0 or later (GPL-3.0-or-later)

Полный текст лицензии: <https://www.gnu.org/licenses/gpl-3.0.txt>

---

## MetaCubeX/meta-rules-dat (.mrs)

- **Репозиторий**: <https://github.com/MetaCubeX/meta-rules-dat> (ветка `meta`)
- **Использование**: скомпилированные rule-set (`.mrs`) для ядра mihomo;
  URL записываются в конфигурацию, загрузку выполняет сам процесс mihomo.
- **Лицензия**: GNU General Public License v3.0 (GPL-3.0)
- **Примечание**: по README агрегирует данные Loyalsoldier,
  v2fly/domain-list-community и MaxMind GeoLite2 (© MaxMind, Inc.).

Полный текст лицензии: <https://www.gnu.org/licenses/gpl-3.0.txt>

---

## 1andrevich/Re-filter-lists

- **Репозиторий**: <https://github.com/1andrevich/Re-filter-lists>
- **Использование**: скомпилированные rule-set из releases; URL записываются
  пресетами Routing Editor в `routing.json`, загрузку выполняет процесс
  sing-box/mihomo.
- **Лицензия**: MIT — Copyright (c) 2024 Andrevich

Текст лицензии MIT идентичен приведённому в разделе bol-van/zapret2
(с заменой copyright-строки).

---

## Flowseal/zapret-discord-youtube (hostlists)

- **Репозиторий**: <https://github.com/Flowseal/zapret-discord-youtube>
- **Использование**: текстовые списки `list-general.txt`/`list-google.txt`
  загружаются в рантайме как источник для `nfqws2 --hostlist`
  (`internal/dpi/update.go`).
- **Лицензия**: MIT — Copyright (c) 2016-2026 bol-van,
  Copyright (c) 2024-2026 Flowseal

Текст лицензии MIT идентичен приведённому в разделе bol-van/zapret2
(с заменой copyright-строк).

---

## MetaCubeX/metacubexd (веб-дашборд, в коде проекта — «Zashboard»)

- **Репозиторий**: <https://github.com/MetaCubeX/metacubexd>
  (git submodule `internal/web/assets/zashboard`, ветка `gh-pages` —
  собранные ассеты)
- **Использование**: статические ассеты дашборда встраиваются в бинарь
  Sign-Craze через `//go:embed` и раздаются веб-сервером `:9090`.
- **Лицензия**: MIT

```plain
MIT License

Copyright (c) 2023 MetaCubeX

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

В составе собранного бандла дашборда дополнительно распространяются:

- **Tailwind CSS v4.2.4** — MIT License, <https://tailwindcss.com>
  (баннер сохранён в CSS-бандле);
- **Шрифты Ubuntu** (`_fonts/*.woff2`) — Ubuntu Font Licence Version 1.0,
  © Canonical Ltd., <https://ubuntu.com/legal/font-licence>;
- **Twemoji Mozilla** (`TwemojiMozilla-flags.woff2`) — сборка шрифта:
  Apache License 2.0, Copyright 2016-2018, Mozilla Foundation
  (<https://github.com/mozilla/twemoji-colr>); графика Twemoji —
  CC-BY 4.0, © Twitter, Inc. и другие участники.

---

## Preact и preact/hooks

- **Репозиторий**: <https://github.com/preactjs/preact>
- **Версия**: 10.x (вендорено с CDN jsDelivr, ADR-0014)
- **Использование**: минифицированные ESM-модули
  `internal/web/assets/routingui/vendor/preact.module.js` и
  `preact-hooks.module.js` встраиваются в бинарь через `//go:embed`
  и раздаются Routing Editor (`:9092`).
- **Лицензия**: MIT — Copyright (c) 2015-present Jason Miller

Текст лицензии MIT идентичен приведённому в разделе
MetaCubeX/metacubexd (с заменой copyright-строки).
Полный текст: <https://github.com/preactjs/preact/blob/main/LICENSE>

---

## htm

- **Репозиторий**: <https://github.com/developit/htm>
- **Версия**: 3.1 (вендорено с CDN jsDelivr, ADR-0014)
- **Использование**: минифицированный ESM-модуль
  `internal/web/assets/routingui/vendor/htm.module.js` встраивается
  в бинарь через `//go:embed` (Routing Editor, `:9092`).
- **Лицензия**: Apache License 2.0 — Copyright 2018 Google Inc.

```plain
Copyright 2018 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```

Полный текст лицензии: <https://www.apache.org/licenses/LICENSE-2.0.txt>

---

## Pico CSS

- **Репозиторий**: <https://github.com/picocss/pico>
- **Версия**: v2.0.6
- **Использование**: `internal/web/assets/routingui/vendor/pico.min.css`
  встраивается в бинарь через `//go:embed` (Routing Editor, `:9092`);
  license-баннер сохранён в файле.
- **Лицензия**: MIT — Copyright (c) 2019-2024 Pico

Текст лицензии MIT идентичен приведённому в разделе
MetaCubeX/metacubexd (с заменой copyright-строки).
Полный текст: <https://github.com/picocss/pico/blob/main/LICENSE.md>

---

## UPX (стаб в упакованных бинарях)

- **Репозиторий**: <https://github.com/upx/upx>
- **Использование**: release-бинари Sign-Craze упаковываются UPX;
  распаковочный стаб UPX является частью каждого упакованного файла.
- **Лицензия**: GPL-2.0-or-later со специальным исключением (UPX License
  Agreement), которое разрешает свободное использование и распространение
  упакованных программ без дополнительных лицензионных ограничений.

Полный текст: <https://github.com/upx/upx/blob/devel/LICENSE>

---

> **Примечание.** Этот файл обновляется вручную при добавлении новых
> зависимостей. Последняя актуализация: 07.07.2026.
