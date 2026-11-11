// Puente mínimo hacia EventKit (Recordatorios). Cada función devuelve un JSON
// reservado con malloc: {"error":"…"} o {"data":…}. Quien llama lo libera con free().
#ifndef TASKTUI_EVENTKIT_H
#define TASKTUI_EVENTKIT_H

char *ek_open(const char *listName);
char *ek_fetch(void);
char *ek_create(const char *json);
char *ek_update(const char *ekID, const char *json);
char *ek_remove(const char *ekID);
char *ek_commit(void);

#endif
