# Changelog

## [1.1.1](https://github.com/tarciosaraiva/pic-bridge/compare/v1.1.0...v1.1.1) (2026-11-13)


### Bug Fixes

* lock destination against concurrent runs ([8dd35db](https://github.com/tarciosaraiva/pic-bridge/commit/8dd35dbc62721c2c54e90e54b968f7788e409f8d))
* lock destination against concurrent runs ([c859ea7](https://github.com/tarciosaraiva/pic-bridge/commit/c859ea72b1110b5059b35857c8aa4444bb3f0657))

## [1.1.0](https://github.com/tarciosaraiva/pic-bridge/compare/v1.0.0...v1.1.0) (2026-11-11)


### Features

* --clean verifies and erases the card after import ([2e0dba4](https://github.com/tarciosaraiva/pic-bridge/commit/2e0dba4426dc3fc5e015863f35abb57b0ba3f8bf))
* --clean verifies and erases the card after import ([3976bb9](https://github.com/tarciosaraiva/pic-bridge/commit/3976bb92a958e76ad3cdc2528ac7d4eff73353c8))
* clean gate re-verifies every card file ([c89b3d9](https://github.com/tarciosaraiva/pic-bridge/commit/c89b3d90250d83cb9fc2e1dcd839370f8575f72b))
* erase card contents after a passed gate ([991de46](https://github.com/tarciosaraiva/pic-bridge/commit/991de46ff58c155a068b30d04381de3c7c637046))
* load full manifest entries for the clean gate ([d986b61](https://github.com/tarciosaraiva/pic-bridge/commit/d986b613772f4e6334ecdf2278a14519e5f83c05))
* verify_source re-reads each card file before placing it ([49a1081](https://github.com/tarciosaraiva/pic-bridge/commit/49a1081a8f49639bff66e91afdf528de228697ce))


### Bug Fixes

* --clean accepts a source with a trailing slash ([61cd037](https://github.com/tarciosaraiva/pic-bridge/commit/61cd0375615c2eb650aeb29650819513b53f94cb))
* --clean removes read-only card files on macOS ([341f42b](https://github.com/tarciosaraiva/pic-bridge/commit/341f42bdb90b7fd54eb74517b2759edb3b99dd35))
* clear only the uchg flag before removing a card file ([7ea3a03](https://github.com/tarciosaraiva/pic-bridge/commit/7ea3a037130fabee349f906009a32533168a9e8d))
* keep the first card read out of the page cache ([134ea77](https://github.com/tarciosaraiva/pic-bridge/commit/134ea77f77b658d7340937f96326a115a74acfda))
* read the card in 1 MB requests ([409780c](https://github.com/tarciosaraiva/pic-bridge/commit/409780cf906bf554a7279cae06ebf292f78b0df0))
* scan logs unreadable card entries and blocks --clean ([858aa1c](https://github.com/tarciosaraiva/pic-bridge/commit/858aa1c74a353a5e874019243d6b6daf80ffe91f))


### Reverts

* flag clearing before removing a card file ([84cf226](https://github.com/tarciosaraiva/pic-bridge/commit/84cf226c54b7e09272425e406be0aa84bfc10d76))

## 1.0.0 (2026-11-09)


### Features

* add --version flag ([3294c6a](https://github.com/tarciosaraiva/pic-bridge/commit/3294c6a8f4b9f63e14646b3d9ddbc8c0cbc67b3e))
* add --version flag ([22cfeca](https://github.com/tarciosaraiva/pic-bridge/commit/22cfeca25c67ab254fe2a1c5a001b5c318544a9f))
* capture date from exif, sony xml, mvhd ([6ddbf5c](https://github.com/tarciosaraiva/pic-bridge/commit/6ddbf5c2a9e5a2c4378e387c7551eaf6d1d7d796))
* card detection and file scan ([00b0f83](https://github.com/tarciosaraiva/pic-bridge/commit/00b0f83831d639248337d2f4db76d491fdedce47))
* config loading and validation ([c94a8d2](https://github.com/tarciosaraiva/pic-bridge/commit/c94a8d25f2cfe1aef639606aede0ca7fa6bc9439))
* import HEIF and split photos/videos in plan line ([93a1709](https://github.com/tarciosaraiva/pic-bridge/commit/93a1709b257ee5c85ba276d5b3645e04734021cd))
* import plan with counters and pairing ([8a9a193](https://github.com/tarciosaraiva/pic-bridge/commit/8a9a1933dab497b53df9ebc2bad0d721e31f692d))
* incremental yaml manifest ([bdd6aea](https://github.com/tarciosaraiva/pic-bridge/commit/bdd6aeac07d6c84bc5b60e180c81eff99f3026b5))
* picb import command ([d23bf8f](https://github.com/tarciosaraiva/pic-bridge/commit/d23bf8f3089e37228d6f828293ec12baee769104))
* run orchestration with group workers and timings ([7c4f6b3](https://github.com/tarciosaraiva/pic-bridge/commit/7c4f6b3cb9d5de71364c75031af6bbe836cad2b9))
* verified copy without overwrite ([3a89b37](https://github.com/tarciosaraiva/pic-bridge/commit/3a89b3770dfde179a0474502a4206a70d2bdb7d9))


### Bug Fixes

* accept flags after the folder name ([85476aa](https://github.com/tarciosaraiva/pic-bridge/commit/85476aaa2676cf7cac4b5c05f03d78e5b907225d))
* finish a started group on Ctrl-C ([f759372](https://github.com/tarciosaraiva/pic-bridge/commit/f759372db66d5ea46240c8f17d38ba7dbf487e28))
* fsync destination folder after placing a file ([7bf1306](https://github.com/tarciosaraiva/pic-bridge/commit/7bf1306beac96f2b4e0f2dcc57ffe5a60a71527a))
* keep EXIF date when imagemeta hits EOF later ([90be8ee](https://github.com/tarciosaraiva/pic-bridge/commit/90be8eed97bc73d2d9ea24e7e372330c49302282))
