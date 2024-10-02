import { createContext } from '@lit/context';
import { Service } from './service';

export const serviceContext = createContext<Service>('service');
