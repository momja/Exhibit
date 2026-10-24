// One application per document. ES modules run once, including when multiple
// page scripts import this entry point. Register new shared controllers here.
import { Application } from '../stimulus/stimulus.js';
import ActionMenuController from './action-menu-controller.js';
import ModalController from './modal-controller.js';

export const application = Application.start();
application.register('action-menu', ActionMenuController);
application.register('modal', ModalController);
