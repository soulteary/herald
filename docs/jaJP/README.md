# ドキュメントインデックス

Herald OTP および検証コードサービスのドキュメントへようこそ。

## 🌐 多言語ドキュメント

- [English](../enUS/README.md) | [中文](../zhCN/README.md) | [Français](../frFR/README.md) | [Italiano](../itIT/README.md) | [日本語](README.md) | [Deutsch](../deDE/README.md) | [한국어](../koKR/README.md)

## 📚 ドキュメント一覧

### コアドキュメント

- **[README.md](../../README.jaJP.md)** - プロジェクト概要とクイックスタートガイド

### 詳細ドキュメント

- **[ARCHITECTURE.md](ARCHITECTURE.md)** - フルスタックアーキテクチャ
  - Stargate / Warden / Herald / Redis / プロバイダー
  - データフローと主要インターフェース（challenge、verify、プロバイダー送信）
  - セキュリティ境界（HMAC/mTLS、PII）

- **[API.md](API.md)** - 完全な API エンドポイントドキュメント
  - 認証方法
  - ヘルスチェックエンドポイント
  - チャレンジの作成と検証
  - レート制限
  - エラーコードとレスポンス

- **[DEPLOYMENT.md](DEPLOYMENT.md)** - デプロイメントガイド
  - Docker Compose デプロイメント
  - 手動デプロイメント
  - 設定オプション
  - 他のサービスとのオプション統合
  - セキュリティのベストプラクティス

- **[MONITORING.md](MONITORING.md)** - モニタリングガイド
  - Prometheus メトリクス
  - Grafana ダッシュボード
  - アラートルール
  - ベストプラクティス

- **[TROUBLESHOOTING.md](TROUBLESHOOTING.md)** - トラブルシューティングガイド
  - 一般的な問題と解決策
  - 診断手順
  - パフォーマンス最適化

## 🚀 クイックナビゲーション

### はじめに

1. [README.jaJP.md](../../README.jaJP.md) を読んでプロジェクトを理解する
2. [クイックスタート](../../README.jaJP.md#クイックスタート) セクションを確認する
3. [設定](../../README.jaJP.md#設定) を参照してサービスを設定する

### 開発者

1. [ARCHITECTURE.md](ARCHITECTURE.md) を読んでフルスタックの全体像を把握する
2. [API.md](API.md) を確認して API インターフェースを理解する
3. [DEPLOYMENT.md](DEPLOYMENT.md) を確認してデプロイメントオプションを理解する

### 運用

1. [DEPLOYMENT.md](DEPLOYMENT.md) を読んでデプロイメント方法を理解する
2. [API.md](API.md) を確認して API エンドポイントの詳細を理解する
3. [セキュリティ](DEPLOYMENT.md#セキュリティ) を参照してセキュリティのベストプラクティスを理解する
4. サービスの健全性を監視: [MONITORING.md](MONITORING.md)
5. 問題をトラブルシューティング: [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## 📖 ドキュメント構造

```
herald/
├── README.md              # プロジェクト主ドキュメント（英語）
├── README.jaJP.md         # プロジェクト主ドキュメント（日本語）
├── docs/
│   ├── enUS/
│   │   ├── README.md       # ドキュメントインデックス（英語）
│   │   ├── ARCHITECTURE.md # フルスタックアーキテクチャ（英語）
│   │   ├── API.md          # API ドキュメント（英語）
│   │   ├── DEPLOYMENT.md   # デプロイメントガイド（英語）
│   │   ├── MONITORING.md   # モニタリングガイド（英語）
│   │   └── TROUBLESHOOTING.md # トラブルシューティングガイド（英語）
│   └── jaJP/
│       ├── README.md       # ドキュメントインデックス（日本語、このファイル）
│       ├── ARCHITECTURE.md # フルスタックアーキテクチャ（日本語）
│       ├── API.md          # API ドキュメント（日本語）
│       ├── DEPLOYMENT.md   # デプロイメントガイド（日本語）
│       ├── MONITORING.md   # モニタリングガイド（日本語）
│       └── TROUBLESHOOTING.md # トラブルシューティングガイド（日本語）
└── ...
```

## 🔍 トピック別検索

### API 関連

- API エンドポイント一覧：[API.md](API.md)
- 認証方法：[API.md#認証](API.md#認証)
- エラー処理：[API.md#エラーコード](API.md#エラーコード)
- レート制限：[API.md#レート制限](API.md#レート制限)

### デプロイメント関連

- Docker デプロイメント：[DEPLOYMENT.md#クイックスタート](DEPLOYMENT.md#クイックスタート)
- 設定オプション：[DEPLOYMENT.md#設定](DEPLOYMENT.md#設定)
- サービス統合：[DEPLOYMENT.md#他のサービスとの統合オプション](DEPLOYMENT.md#他のサービスとの統合オプション)
- セキュリティ：[DEPLOYMENT.md#セキュリティ](DEPLOYMENT.md#セキュリティ)

### モニタリングと運用

- Prometheus メトリクス：[MONITORING.md](MONITORING.md)
- Grafana ダッシュボード：[MONITORING.md#grafana-ダッシュボード](MONITORING.md#grafana-ダッシュボード)
- トラブルシューティング：[TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## 💡 使用推奨事項

1. **初めてのユーザー**：[README.jaJP.md](../../README.jaJP.md) から始めて、クイックスタートガイドに従う
2. **サービスを設定する**：[DEPLOYMENT.md](DEPLOYMENT.md) を参照してすべての設定オプションを理解する
3. **サービスと統合する**：[DEPLOYMENT.md](DEPLOYMENT.md) の統合セクションを確認する
4. **API 統合**：[API.md](API.md) を読んで API インターフェースを理解する
5. **サービスを監視する**：[MONITORING.md](MONITORING.md) でモニタリングを設定する
6. **問題をトラブルシューティング**：[TROUBLESHOOTING.md](TROUBLESHOOTING.md) を参照して一般的な問題を確認する

## 📝 ドキュメント更新

ドキュメントはプロジェクトの進化に伴って継続的に更新されます。エラーを見つけたり、追加が必要な場合は、Issue または Pull Request を送信してください。

## 🤝 貢献

ドキュメントの改善を歓迎します：

1. エラーや改善が必要な領域を見つける
2. 問題を説明する Issue を送信する
3. または直接 Pull Request を送信する
