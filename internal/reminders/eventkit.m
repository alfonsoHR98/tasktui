#import <EventKit/EventKit.h>
#import <Foundation/Foundation.h>
#include "eventkit.h"

static EKEventStore *gStore;
static EKCalendar *gList;

static char *reply(id data, NSString *error) {
    NSDictionary *d = error ? @{@"error": error} : @{@"data": data ?: [NSNull null]};
    NSData *json = [NSJSONSerialization dataWithJSONObject:d options:0 error:nil];
    NSString *s = [[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding];
    return strdup(s.UTF8String);
}

static NSDictionary *parse(const char *json) {
    NSData *d = [NSData dataWithBytes:json length:strlen(json)];
    id o = [NSJSONSerialization JSONObjectWithData:d options:0 error:nil];
    return [o isKindOfClass:[NSDictionary class]] ? o : @{};
}

char *ek_open(const char *listName) {
    @autoreleasepool {
        gStore = [[EKEventStore alloc] init];
        EKAuthorizationStatus status = [EKEventStore authorizationStatusForEntityType:EKEntityTypeReminder];
        if (status != EKAuthorizationStatusFullAccess) {
            if (status == EKAuthorizationStatusDenied || status == EKAuthorizationStatusRestricted) {
                return reply(nil, @"Sin acceso a Recordatorios. Actívalo en Ajustes del Sistema › Privacidad y seguridad › Recordatorios (para tu app de terminal).");
            }
            dispatch_semaphore_t sem = dispatch_semaphore_create(0);
            __block BOOL granted = NO;
            [gStore requestFullAccessToRemindersWithCompletion:^(BOOL g, NSError *e) {
                granted = g;
                dispatch_semaphore_signal(sem);
            }];
            if (dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, 120 * NSEC_PER_SEC)) != 0 || !granted) {
                return reply(nil, @"No se concedió acceso a Recordatorios.");
            }
        }
        NSString *name = [NSString stringWithUTF8String:listName];
        for (EKCalendar *c in [gStore calendarsForEntityType:EKEntityTypeReminder]) {
            if ([c.title isEqualToString:name]) {
                gList = c;
                return reply(@YES, nil);
            }
        }
        EKCalendar *def = [gStore defaultCalendarForNewReminders];
        if (!def) return reply(nil, @"Recordatorios no tiene una lista por defecto");
        EKCalendar *cal = [EKCalendar calendarForEntityType:EKEntityTypeReminder eventStore:gStore];
        cal.title = name;
        cal.source = def.source;
        NSError *err = nil;
        if (![gStore saveCalendar:cal commit:YES error:&err]) return reply(nil, err.localizedDescription);
        gList = cal;
        return reply(@YES, nil);
    }
}

char *ek_fetch(void) {
    @autoreleasepool {
        NSPredicate *pred = [gStore predicateForRemindersInCalendars:@[gList]];
        dispatch_semaphore_t sem = dispatch_semaphore_create(0);
        __block NSArray<EKReminder *> *items = nil;
        [gStore fetchRemindersMatchingPredicate:pred completion:^(NSArray<EKReminder *> *r) {
            items = r;
            dispatch_semaphore_signal(sem);
        }];
        if (dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, 30 * NSEC_PER_SEC)) != 0) {
            return reply(nil, @"Recordatorios no respondió a tiempo");
        }
        NSMutableArray *out = [NSMutableArray array];
        for (EKReminder *r in items) {
            NSDateComponents *c = r.dueDateComponents;
            BOOL hasDate = c && c.year != NSDateComponentUndefined && c.month != NSDateComponentUndefined && c.day != NSDateComponentUndefined;
            BOOL hasTime = c && c.hour != NSDateComponentUndefined;
            [out addObject:@{
                @"id": r.calendarItemIdentifier ?: @"",
                @"title": r.title ?: @"",
                @"hasDate": @(hasDate),
                @"y": @(hasDate ? c.year : 0), @"m": @(hasDate ? c.month : 0), @"d": @(hasDate ? c.day : 0),
                @"hasTime": @(hasTime),
                @"h": @(hasTime ? c.hour : 0),
                @"min": @(hasTime && c.minute != NSDateComponentUndefined ? c.minute : 0),
                @"priority": @(r.priority),
                @"notes": r.notes ?: @"",
                @"completed": @(r.isCompleted),
            }];
        }
        return reply(out, nil);
    }
}

static void fill(EKReminder *r, NSDictionary *w) {
    r.title = w[@"title"];
    NSDateComponents *c = [[NSDateComponents alloc] init];
    c.year = [w[@"y"] integerValue];
    c.month = [w[@"m"] integerValue];
    c.day = [w[@"d"] integerValue];
    BOOL hasTime = [w[@"hasTime"] boolValue];
    if (hasTime) {
        c.hour = [w[@"h"] integerValue];
        c.minute = [w[@"min"] integerValue];
    }
    r.dueDateComponents = c;
    // Con hora, una alarma a esa hora; sin hora, Recordatorios avisa a la hora general de «todo el día».
    r.alarms = nil;
    if (hasTime) {
        NSDate *at = [[NSCalendar currentCalendar] dateFromComponents:c];
        if (at) [r addAlarm:[EKAlarm alarmWithAbsoluteDate:at]];
    }
    r.priority = [w[@"priority"] unsignedIntegerValue];
    r.notes = w[@"notes"];
    if (r.isCompleted) r.completed = NO;
}

static EKReminder *item(const char *ekID) {
    EKCalendarItem *i = [gStore calendarItemWithIdentifier:[NSString stringWithUTF8String:ekID]];
    return [i isKindOfClass:[EKReminder class]] ? (EKReminder *)i : nil;
}

char *ek_create(const char *json) {
    @autoreleasepool {
        EKReminder *r = [EKReminder reminderWithEventStore:gStore];
        r.calendar = gList;
        fill(r, parse(json));
        NSError *err = nil;
        if (![gStore saveReminder:r commit:NO error:&err]) return reply(nil, err.localizedDescription);
        return reply(r.calendarItemIdentifier, nil);
    }
}

char *ek_update(const char *ekID, const char *json) {
    @autoreleasepool {
        EKReminder *r = item(ekID);
        if (!r) return reply(@YES, nil);
        fill(r, parse(json));
        NSError *err = nil;
        if (![gStore saveReminder:r commit:NO error:&err]) return reply(nil, err.localizedDescription);
        return reply(@YES, nil);
    }
}

char *ek_remove(const char *ekID) {
    @autoreleasepool {
        EKReminder *r = item(ekID);
        if (!r) return reply(@YES, nil);
        NSError *err = nil;
        if (![gStore removeReminder:r commit:NO error:&err]) return reply(nil, err.localizedDescription);
        return reply(@YES, nil);
    }
}

char *ek_commit(void) {
    @autoreleasepool {
        NSError *err = nil;
        if (![gStore commit:&err]) return reply(nil, err.localizedDescription);
        return reply(@YES, nil);
    }
}
