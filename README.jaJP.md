# Herald - OTP および検証コードサービス

> **📧 安全な検証へのゲートウェイ**

## 🌐 多言語ドキュメント

- [English](README.md) | [中文](README.zhCN.md) | [Français](README.frFR.md) | [Italiano](README.itIT.md) | [日本語](README.jaJP.md) | [Deutsch](README.deDE.md) | [한국어](README.koKR.md)

![Herald](.github/assets/banner.jpg)

Herald は、電子メール経由で検証コード（OTP）を送信するための本番対応の軽量サービスです（SMS サポートは現在開発中）。レート制限、セキュリティ制御、監査ログ記録が組み込まれています。

## 機能

- 🚀 **高性能**：Go と Fiber で構築
- 🔒 **セキュア**：ハッシュストレージを使用したチャレンジベースの検証
- 📊 **レート制限**：多次元レート制限（ユーザーごと、IP ごと、宛先ごと）
- 📝 **監査ログ**：すべての操作の完全な監査証跡
- 🔌 **プラガブルプロバイダー**：電子メールプロバイダーのサポート（SMS プロバイダーはプレースホルダー実装で、まだ完全には機能していません）
- ⚡ **Redis バックエンド**：Redis を使用した高速で分散されたストレージ

## クイックスタート

```bash
# Docker Compose で実行
docker-compose up -d

# または直接実行
go run main.go
```

## 設定

環境変数を設定：

- `PORT`：サーバーポート（デフォルト：`:8082`）
- `REDIS_ADDR`：Redis アドレス（デフォルト：`localhost:6379`）
- `REDIS_PASSWORD`：Redis パスワード（オプション）
- `REDIS_DB`：Redis データベース番号（デフォルト：`0`）
- `API_KEY`：サービス間認証用の API キー
- `LOG_LEVEL`：ログレベル（デフォルト：`info`）

完全な設定オプションについては、[DEPLOYMENT.md](docs/jaJP/DEPLOYMENT.md) を参照してください。

## API ドキュメント

詳細な API ドキュメントについては、[API.md](docs/jaJP/API.md) を参照してください。
