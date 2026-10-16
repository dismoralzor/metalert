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

## Оптимизация памяти (инкремент 17)

Профилировался самый аллоцирующий горячий путь — рендер HTML-страницы со всеми
метриками (`GET /`, хендлер `IndexHandler.Index`). Профили сняты одним и тем же
бенчмарком `BenchmarkIndexHandler` (500 gauge + 500 counter) с фиксированным
числом итераций `-benchtime=10000x`, чтобы diff профилей был сопоставим:

```
go test -run=^$ -bench=BenchmarkIndexHandler -benchmem -benchtime=10000x -memprofile=profiles/base.pprof   ./internal/handler   # до
go test -run=^$ -bench=BenchmarkIndexHandler -benchmem -benchtime=10000x -memprofile=profiles/result.pprof ./internal/handler   # после
```

### Что оптимизировано

Исходный `Index` рендерил страницу через `html/template`, а значения складывал
в промежуточные срезы `[]metricRow` и буфер, аллоцируемый с нуля на каждый
запрос. Профиль `base` показал, что подавляющая часть аллокаций приходится на
рефлексию `html/template` (`reflect.Value.call`, `reflect.MakeSlice`,
`reflect.packEface`, `html/template.htmlReplacer`) и на рост `bytes.Buffer`.

Изменения (внешнее поведение и сигнатуры сохранены — страница та же, тесты
зелёные):

1. Убран `html/template`: страница собирается прямыми записями в буфер, имена
   метрик экранируются через `html.EscapeString` — это сняло весь рефлексивный
   аппарат шаблонизатора.
2. Убраны промежуточные срезы `[]metricRow` и структура `data` — значения
   пишутся в буфер напрямую.
3. Буфер рендера переиспользуется между запросами через `sync.Pool` вместо
   аллокации с нуля (убрало рост `bytes.Buffer`).

### Результат (benchmem, на операцию)

| Метрика    | База (base) | После (result) | Изменение |
|------------|-------------|-----------------|-----------|
| B/op       | 506218      | 104117          | −79 %     |
| allocs/op  | 14433       | 1410            | −90 %     |
| ns/op      | 1594255     | 94334           | −94 %     |

### Diff профилей (`go tool pprof -diff_base`)

Все значения отрицательные — память и число аллокаций уменьшились.

```
$ go tool pprof -top -sample_index=alloc_space -diff_base=profiles/base.pprof profiles/result.pprof
      flat  flat%   sum%        cum   cum%
 -732.51MB 15.24% 15.24% -2400.55MB 49.95%  reflect.Value.call
 -720.95MB 15.00% 30.24% -3785.21MB 78.76%  (*IndexHandler).Index
 -593.52MB 12.35% 42.59%  -593.52MB 12.35%  html/template.htmlReplacer
 -584.07MB 12.15% 54.74%  -584.07MB 12.15%  bytes.growSlice
 -473.01MB  9.84% 64.58%  -771.02MB 16.04%  reflect.MakeSlice
 -303.50MB  6.31% 70.90%  -303.50MB  6.31%  reflect.packEface
    -298MB  6.20% 77.10%     -298MB  6.20%  reflect.unsafe_NewArray

$ go tool pprof -top -sample_index=alloc_objects -diff_base=profiles/base.pprof profiles/result.pprof
      flat  flat%   sum%        cum   cum%
 -38382983 29.79% 29.79% -117918047 91.51%  reflect.Value.call
 -20666158 16.04% 45.83%  -40196184 31.20%  reflect.MakeSlice
 -19890479 15.44% 61.26%  -19890479 15.44%  reflect.packEface
 -19530026 15.16% 76.42%  -19530026 15.16%  reflect.unsafe_NewArray
 -19448401 15.09% 91.51%  -19448401 15.09%  html/template.htmlReplacer
   -247814  0.19% 95.09% -123090121 95.53%  (*IndexHandler).Index
```

Профили `profiles/base.pprof` и `profiles/result.pprof` генерируются командами
выше в каталог `profiles/` (он в `.gitignore` и в репозитории не хранится).
