/* eslint-disable @typescript-eslint/no-unused-vars */

import { TestBed, tick, fakeAsync, flush } from '@angular/core/testing';
import { TranslateService, TranslateLoader, TranslateParser, TranslateModule } from '@ngx-translate/core';
import { RouterTestingModule } from '@angular/router/testing';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { Observable, of } from 'rxjs';
import { GroupService } from '../../../service/group/group.service';
import { Variable } from '../../../model/variable.model';
import { VariableEvent } from '../variable.event.model';
import { SharedModule } from '../../shared.module';
import { SharedService } from '../../shared.service';
import { VariableService } from '../../../service/variable/variable.service';
import { VariableFormComponent } from './variable.form';
import { BrowserAnimationsModule } from '@angular/platform-browser/animations';
import { provideHttpClient, withInterceptorsFromDi } from '@angular/common/http';

describe('CDS: Variable From Component', () => {

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            declarations: [],
            providers: [
                { provide: VariableService, useClass: MockApplicationService },
                GroupService,
                SharedService,
                TranslateService,
                TranslateLoader,
                TranslateParser,
                provideHttpClient(withInterceptorsFromDi()),
                provideHttpClientTesting()
            ],
            imports: [
                BrowserAnimationsModule,
                SharedModule,
                TranslateModule.forRoot(),
                RouterTestingModule.withRoutes([])
            ]
        }).compileComponents();
    });


    it('Create new variable', fakeAsync(() => {

        // Create component
        let fixture = TestBed.createComponent(VariableFormComponent);
        let component = fixture.debugElement.componentInstance;
        expect(component).toBeTruthy();

        fixture.detectChanges();
        tick(50);

        expect(fixture.debugElement.nativeElement.querySelector('button[name="saveBtn"][disabled="true"]')).toBeTruthy();

        let compiled = fixture.debugElement.nativeElement;

        let variable = new Variable();
        variable.name = 'foo';
        variable.type = 'string';
        variable.value = 'bar';

        fixture.detectChanges();
        tick(50);

        // simulate typing new variable
        let inputName = compiled.querySelector('input[name="name"]');
        inputName.value = variable.name;
        inputName.dispatchEvent(new Event('input'));

        fixture.componentInstance.newVariable.type = variable.type;

        fixture.detectChanges();
        tick(50);

        let inputValue = compiled.querySelector('input[name="value"]');
        inputValue.value = variable.value;
        inputValue.dispatchEvent(new Event('input'));
        inputValue.dispatchEvent(new Event('change'));

        spyOn(fixture.componentInstance.createVariableEvent, 'emit');
        compiled.querySelector('button[name="saveBtn"]').click();

        expect(fixture.componentInstance.createVariableEvent.emit).toHaveBeenCalledWith(new VariableEvent('add', variable));

        flush()
    }));
});

class MockApplicationService {
    constructor() { }

    getTypesFromCache(): string[] {
        return [];
    }

    getTypesFromAPI(): Observable<string[]> {
        return of(['string', 'password']);
    }
}
