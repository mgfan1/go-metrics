// Package handler реализует HTTP API сервера метрик.
//
// Маршруты:
//
//	POST /update/{type}/{name}/{value}  обновить метрику из адреса запроса
//	POST /update/                       обновить метрику из JSON
//	POST /updates/                      обновить батч метрик из JSON
//	GET  /value/{type}/{name}           значение метрики текстом
//	POST /value/                        значение метрики в JSON
//	GET  /                              HTML-страница со всеми метриками
//	GET  /ping                          проверка соединения с базой
package handler
