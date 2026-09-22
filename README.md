# generate-google-cred

CLI на Go для интерактивного получения пользовательского Google OAuth 2.0 token-файла. Командная строка построена на Cobra, конфигурация загружается через Viper. Параметры можно передавать флагами, переменными окружения или YAML-файлом.

## Подготовка Google OAuth

1. В Google Cloud Console создайте OAuth client ID типа **Web application**.
2. Добавьте точный redirect URI `http://localhost:8080/callback` (или URI из вашей конфигурации).
3. Скачайте JSON клиента и сохраните локально как `client_secret.json`.
4. Настройте scopes в `scopes.txt`, YAML-конфиге или флагах `--scope`.


## Сборка

Требуется Go 1.26 или новее.

```bash
go build -o generate-google-cred .
```

Или скачайте архив для своей ОС из GitHub Releases.

## Запуск через флаги

```bash
./generate-google-cred \
  --client-secret ./client_secret.json \
  --token-output ./token.json \
  --scopes-file ./scopes.txt \
  --redirect-url http://localhost:8080/callback \
  --listen-address 127.0.0.1:8080
```

Scopes можно передать без файла:

```bash
./generate-google-cred \
  --scopes-file "" \
  --scope openid \
  --scope https://www.googleapis.com/auth/userinfo.email
```

После запуска CLI поднимает локальный callback-сервер, печатает ссылку и открывает браузер. После согласия пользователя access token и refresh token атомарно записываются в файл с правами `0600`.

Все доступные параметры:

```bash
./generate-google-cred --help
```

Для сервера без GUI используйте `--open-browser=false` и откройте напечатанную ссылку вручную на машине, которая может обратиться к указанному callback URL.

## Запуск через YAML

Скопируйте пример и измените значения:

```bash
cp config.example.yaml config.yaml
./generate-google-cred
```

Файл `config.yaml` в текущем каталоге загружается автоматически. Другой файл можно указать явно:

```bash
./generate-google-cred --config ./configs/google.yaml
```

Пример структуры находится в [`config.example.yaml`](config.example.yaml). Scopes из `scopes` и `scopes-file` объединяются, дубликаты удаляются. Значение, переданное флагом, имеет приоритет над YAML. Также поддерживаются переменные окружения с префиксом `GGC_`, например `GGC_TOKEN_OUTPUT` и `GGC_OPEN_BROWSER`.

## Безопасность OAuth

CLI использует случайный `state` и PKCE для защиты callback, запрашивает offline access и по умолчанию показывает consent screen, чтобы Google вернул refresh token. Отключить повторный consent можно флагом `--force-consent=false`.

Redirect URI должен в точности совпадать со значением в Google Cloud, включая схему, host, port и path. По умолчанию сервер слушает только loopback-адрес `127.0.0.1:8080`.

## Проверки и релизы

GitHub Actions запускает:

- тесты с race detector, `go vet`, сборку и `golangci-lint` для push/PR;
- повторные проверки для тегов `v*`;
- кросс-компиляцию Linux, macOS и Windows для `amd64` и `arm64`;
- создание GitHub Release, загрузку архивов и `checksums.txt`.

Для выпуска новой версии создайте и отправьте тег:

```bash
git tag v1.0.0
git push origin v1.0.0
```

Workflow использует встроенный `GITHUB_TOKEN`; дополнительные secrets не требуются.
