# go-musthave-metrics-tpl

Шаблон репозитория для трека «Сервер сбора метрик и алертинга».

## Начало работы

1. Склонируйте репозиторий в любую подходящую директорию на вашем компьютере.
2. В корне репозитория выполните команду `go mod init <name>` (где `<name>` — адрес вашего репозитория на GitHub без префикса `https://`) для создания модуля.

## Обновление шаблона

Чтобы иметь возможность получать обновления автотестов и других частей шаблона, выполните команду:

```
git remote add -m v2 template https://github.com/Yandex-Practicum/go-musthave-metrics-tpl.git
```

Для обновления кода автотестов выполните команду:

```
git fetch template && git checkout template/v2 .github
```

Затем добавьте полученные изменения в свой репозиторий.

## Запуск автотестов

Для успешного запуска автотестов называйте ветки `iter<number>`, где `<number>` — порядковый номер инкремента. Например, в ветке с названием `iter4` запустятся автотесты для инкрементов с первого по четвёртый.

При мёрже ветки с инкрементом в основную ветку `main` будут запускаться все автотесты.

Подробнее про локальный и автоматический запуск читайте в [README автотестов](https://github.com/Yandex-Practicum/go-autotests).

## Структура проекта

Приведённая в этом репозитории структура проекта является рекомендуемой, но не обязательной.

Это лишь пример организации кода, который поможет вам в реализации сервиса.

При необходимости можно вносить изменения в структуру проекта, использовать любые библиотеки и предпочитаемые структурные паттерны организации кода приложения, например:
- **DDD** (Domain-Driven Design)
- **Clean Architecture**
- **Hexagonal Architecture**
- **Layered Architecture**

## Профилирование памяти

Профиль снимается бенчмарками полного HTTP-пути сервера из `internal/handler/bench_test.go`. В них работает роутер со всеми middleware и хранилищем в памяти. Запросы те же, что приходят на сервер: батч агента из 35 метрик со сжатием gzip и подписью, обновление одной метрики в JSON, чтение значения и страница со списком метрик. Число итераций фиксировано, поэтому профили до и после правок можно сравнивать напрямую.

```
go test ./internal/handler -run '^$' -bench . -benchtime 2000x -memprofilerate 1 -memprofile profiles/base.pprof
```

В `profiles/base.pprof` 92% всей выделенной памяти приходилось на `compress/flate.NewWriter`. Это видно в `go tool pprof -top`, а `-peek` и `-list` показали источник: middleware gzip создавал новый компрессор на каждый ответ, около 800 КБ за раз. Ещё 2,6% давал `gzip.NewReader` на каждый сжатый запрос.

Что исправлено:

- Компрессоры и распаковщики gzip теперь берутся из `sync.Pool` и сбрасываются через `Reset`. Так сделано и в middleware сервера, и в агенте при отправке батча.
- Буферы для подписи в `HashSign` и `HashCheck` тоже берутся из пула. Буфер больше 64 КБ обратно в пул не кладётся, чтобы один большой запрос не держал память.

После правок тот же прогон сохранён в `profiles/result.pprof`. Сравнение:

```
go tool pprof -top -diff_base=profiles/base.pprof profiles/result.pprof
```

```
Type: alloc_space
Time: 2026-11-12 03:08:21 +07
Showing nodes accounting for -3202858.86kB, 93.83% of 3413564.19kB total
Dropped 165 nodes (cum <= 17067.82kB)
      flat  flat%   sum%        cum   cum%
-2558953.97kB 74.96% 74.96% -3111321.22kB 91.15%  compress/flate.NewWriter (inline)
-537064.88kB 15.73% 90.70% -537064.88kB 15.73%  compress/flate.(*compressor).initDeflate (inline)
-63744.11kB  1.87% 92.56% -63744.11kB  1.87%  compress/flate.(*dictDecoder).init (inline)
-26656.30kB  0.78% 93.35% -26656.30kB  0.78%  compress/flate.(*huffmanEncoder).generate
-15002.25kB  0.44% 93.79% -78746.41kB  2.31%  compress/flate.NewReader
   -1375kB  0.04% 93.83% -88625.25kB  2.60%  compress/gzip.NewReader (inline)
  -62.50kB 0.0018% 93.83% -1564375.99kB 45.83%  github.com/mgfan1/go-metrics/internal/handler.(*MetricsHandler).Router.HashSign.func5.1
    0.09kB 2.7e-06% 93.83% -78746.27kB  2.31%  compress/gzip.(*Reader).readHeader
    0.05kB 1.4e-06% 93.83% -1669154.78kB 48.90%  github.com/mgfan1/go-metrics/internal/handler.BenchmarkUpdatesFromAgent
         0     0% 93.83% -26656.30kB  0.78%  compress/flate.(*Writer).Close (inline)
         0     0% 93.83% -26656.30kB  0.78%  compress/flate.(*compressor).close
         0     0% 93.83% -26656.30kB  0.78%  compress/flate.(*compressor).deflate
         0     0% 93.83% -552367.25kB 16.18%  compress/flate.(*compressor).init
         0     0% 93.83% -26656.30kB  0.78%  compress/flate.(*compressor).writeBlock
         0     0% 93.83% -17770.94kB  0.52%  compress/flate.(*huffmanBitWriter).indexTokens
         0     0% 93.83% -26656.30kB  0.78%  compress/flate.(*huffmanBitWriter).writeBlock
         0     0% 93.83% -86933.77kB  2.55%  compress/gzip.(*Reader).Reset
         0     0% 93.83% -26656.30kB  0.78%  compress/gzip.(*Writer).Close
         0     0% 93.83% -3111321.22kB 91.15%  compress/gzip.(*Writer).Write
         0     0% 93.83% -3248553.20kB 95.17%  github.com/go-chi/chi/v5.(*Mux).ServeHTTP
         0     0% 93.83% -1573765.10kB 46.10%  github.com/go-chi/chi/v5.(*Mux).routeHTTP
         0     0% 93.83% -1566064.20kB 45.88%  github.com/mgfan1/go-metrics/internal/handler.(*MetricsHandler).List
         0     0% 93.83% -16740.69kB  0.49%  github.com/mgfan1/go-metrics/internal/handler.(*MetricsHandler).Router.HashCheck.func6.1
         0     0% 93.83% -3245197.38kB 95.07%  github.com/mgfan1/go-metrics/internal/handler.(*MetricsHandler).Router.Logging.func4.1
         0     0% 93.83% -1582822.27kB 46.37%  github.com/mgfan1/go-metrics/internal/handler.BenchmarkListPage
         0     0% 93.83% -3248553.20kB 95.17%  github.com/mgfan1/go-metrics/internal/handler.serveOK
         0     0% 93.83% -26574.32kB  0.78%  github.com/mgfan1/go-metrics/internal/middleware.(*gzipWriter).Close (partial-inline)
         0     0% 93.83% -3111610.19kB 91.15%  github.com/mgfan1/go-metrics/internal/middleware.(*gzipWriter).Write
         0     0% 93.83% -1547637.45kB 45.34%  github.com/mgfan1/go-metrics/internal/middleware.(*hashWriter).flush
         0     0% 93.83% -3245197.38kB 95.07%  github.com/mgfan1/go-metrics/internal/middleware.Gzip.func1
         0     0% 93.83% -34000.55kB     1%  github.com/mgfan1/go-metrics/internal/middleware.Gzip.func1.2
         0     0% 93.83% -1566065.05kB 45.88%  html/template.(*Template).Execute
         0     0% 93.83% -3245197.38kB 95.07%  net/http.HandlerFunc.ServeHTTP
         0     0% 93.83% -3251975.59kB 95.27%  testing.(*B).run1.func1
         0     0% 93.83% -3251975.46kB 95.27%  testing.(*B).runN
         0     0% 93.83% -1566065.05kB 45.88%  text/template.(*Template).Execute (inline)
         0     0% 93.83% -1566065.05kB 45.88%  text/template.(*Template).execute
         0     0% 93.83% -1566065.05kB 45.88%  text/template.(*state).walk
```

Бенчмарки до и после, `go test ./internal/... -run '^$' -bench . -benchmem`. Время на операцию сильно зависит от загрузки машины, поэтому в таблице только память:

| Бенчмарк | B/op до | B/op после | allocs/op до | allocs/op после |
|---|---|---|---|---|
| `handler` UpdatesFromAgent | 891 851 | 29 384 | 217 | 183 |
| `handler` ListPage | 838 899 | 25 573 | 518 | 498 |
| `middleware` Gzip | 869 332 | 7 907 | 53 | 27 |
| `middleware` Hash | 16 680 | 9 954 | 53 | 46 |
| `agent` Send | 867 795 | 27 220 | 144 | 105 |

Остаток в `profiles/result.pprof` примерно поровну делят тестовая обвязка `httptest` и компрессоры, которые пул создаёт заново после очистки сборщиком мусора.
