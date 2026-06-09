/*
 * COMMAND14.C:
 *
 *      Additional user routines.
 *
 *      Copyright (C) 1991, 1992, 1993 Brett J. Vickers
 *
 */
#include <stdlib.h>
#include "mstruct.h"
#include "mextern.h"
#include "mtype.h"

/*++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++*/
/*                  엄호                                        */
/*++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++*/
long ply_guard_time[PMAX];
int guard(ply_ptr, cmnd)
creature    *ply_ptr;
cmd     *cmnd;
{
  creature    *crt_ptr;
  room        *rom_ptr;
  room        *new_rom;
  exit_       *ext_ptr;
  char        file[80];
  ctag        *cp;
  int     fd, t, chance, dmg, m=0, j, k=0, enm_thaco=0, q, addprof;
  fd = ply_ptr->fd;
  if(fd < 0) return(0);
  
  if(ply_ptr->class < INVINCIBLE && !(ply_ptr->class == RANGER && ply_ptr->level >= 50)) {

	 ANSI(fd, CYAN);
	 print(fd, "포졸");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, " 레벨 ");
	 ANSI(fd, CYAN);
	 print(fd, "50");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이상만 쓸수 있는 기술입니다.\n");
	 return(0);

  }
  if(ply_ptr->class >= INVINCIBLE && !S_ISSET(ply_ptr, SRANGER)) {

	 print(fd, "아직 ");
	 ANSI(fd, CYAN);
	 print(fd, "포졸");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "을 ");
	 ANSI(fd, CYAN);
	 print(fd, "무적수련");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "하지 않았습니다.\n");
	 return(0);

  }
  
  if(cmnd->num < 3) {
    print(fd, "\n사용법 : 어디 누구 엄호\n");
    return(0);
  }
  if(F_ISSET(ply_ptr, PBLIND)) {
    ANSI(fd, BOLD);
    ANSI(fd, RED);
    print(fd, "당신은 눈이 멀어 있습니다!");
    ANSI(fd, WHITE);
    ANSI(fd, NORMAL);
    return(0);
  }
  
  t = time(0);
  
  if(ply_guard_time[fd] > t) {
    please_wait(fd, ply_guard_time[fd] -t);
    return(0);
  }
  rom_ptr = ply_ptr->parent_rom;
  
  ext_ptr=find_ext(ply_ptr, rom_ptr->first_ext, cmnd->str[1], cmnd->val[1]);
  
  if(ext_ptr) {
    if(F_ISSET(ext_ptr, XCLOSD)) {
      print(fd, "그 출구는 닫혀 있습니다.");
      return(0);
    }
    sprintf(file, "%s/r%02d/r%05d", ROOMPATH,ext_ptr->room/1000, ext_ptr->room);
    if(!file_exists(file)) {
      print(fd, "지도가 없습니다.");
      return(0);
    }
    load_rom(ext_ptr->room, &new_rom);
    if(!new_rom || rom_ptr == new_rom) {
      print(fd, "지도가 없습니다.");
      return(0);
    }
    if(F_ISSET(new_rom, RONMAR) || F_ISSET(new_rom, RONFML)) {
      print(fd, "그 방은 볼 수가 없습니다.");
      return(0);
    }
  }
  else {
    print(fd,"\n%s쪽으로는 지도가 없습니다.", cmnd->str[1]);
    return(0);
  }
  
  crt_ptr = find_crt(ply_ptr, new_rom->first_ply, cmnd->str[2], cmnd->val[2]);
  
  if(!crt_ptr) {
    crt_ptr = find_who(cmnd->str[2]);
    if(crt_ptr) 
      print(fd, "\n%s쪽에 %M은 존재하지 않습니다.\n", ext_ptr->name, crt_ptr);
    else
      print(fd, "\n%s쪽에 %s님은 존재하지 않습니다.\n", ext_ptr->name, cmnd->str[2]);
    return(0);
  }
  
  if(!ply_ptr->ready[WIELD-1] || (ply_ptr->ready[WIELD-1]->type != MISSILE )) {

	 ANSI(fd, YELLOW);
	 print(fd, "엄호");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "를 구사하시려면 ");
	 ANSI(fd, RED);
	 print(fd, "활종류");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "의 무기가 필요합니다.");
	 return(0);

  }
  
  cp = new_rom->first_mon;
  while(cp) {
    
    if((F_ISSET(ply_ptr, PDINVI) ? 1:!F_ISSET(cp->crt, MINVIS)) && !F_ISSET(cp->crt, MHIDDN) 
       && !F_ISSET(cp->crt, MUNKIL )) {
      
      m++;
      enm_thaco += (20 - cp->crt->thaco);
      add_enm_crt(ply_ptr->name, cp->crt);
    }
    cp = cp->next_tag;
  }
  
  if(m<1) {
    print(fd,"\n%M 주위에 당신이 공격할 적이 없습니다.",crt_ptr);
    return(0);
  }
  
  ply_ptr->lasttime[LT_ATTCK].ltime = t;

  print(fd, "\n당신은 활시위를 당겨 %M의 주위의 적에게 공격을 가합니다.\n", crt_ptr);
  
  chance = (20- ply_ptr->thaco) - (enm_thaco/m)*2 + ply_ptr->level/10 + bonus[ply_ptr->dexterity]*5;
  chance = MIN(chance, 20);
  if(chance < 3) chance = 3;
  
  if (mrand(1,22) <= chance) {
    
    k = MIN(chance/5, m);
    cp = new_rom->first_mon;
    for( j=0 ;j<k;j++)  {
      if((F_ISSET(ply_ptr, PDINVI) ? 1:!F_ISSET(cp->crt, MINVIS)) && !F_ISSET(cp->crt, MHIDDN) 
	 && !F_ISSET(cp->crt, MUNKIL )) {
	dmg = (mrand(1, ply_ptr->dexterity)) + mdice(ply_ptr->ready[WIELD-1]);
	dmg = MIN(cp->crt->hpcur, dmg);
	 if(ply_ptr->ready[WIELD-1]) {
	   q = MIN(ply_ptr->ready[WIELD-1]->type, MISSILE);
	   addprof = (dmg * cp->crt->experience) / (cp->crt->hpmax * k);
	   addprof = MIN(addprof, cp->crt->experience);
	   ply_ptr->proficiency[q] += addprof;
	 }
	
	if(mrand(0,2) == 1 && (ply_ptr->strength < (100-(cp->crt->armor)/10)) && F_ISSET(cp->crt, 
MMGONL)) {
	  print(fd, "당신의 활이 %M에게 아무런 상처도 내지 못합니다.\n", cp->crt);
	  dmg = 1;
	}
	if(F_ISSET(cp->crt, MENONL) && ply_ptr->ready[WIELD-1]->adjustment < 1) {
	  print(fd, "당신의 활이 %M의 갑옷을 뚫기엔 역부족입니다.\n", cp->crt);
	  dmg = 1;
	}
	
	print(fd, "\n당신은 %M 주위의 %m에게 %d의 피해를 입힙니다.\n", crt_ptr, cp->crt, dmg);  
	broadcast_rom(fd, ply_ptr->rom_num,
		      "\n%M이 %M을 엄호하여 %m에게 %d의 피해를 입혔습니다.\n", 
		      ply_ptr, crt_ptr, cp->crt , dmg);
	broadcast_rom(fd, crt_ptr->rom_num,
		      "\n%M이 %M을 엄호하여 %m에게 %d의 피해를 입혔습니다.\n", 
		      ply_ptr, crt_ptr, cp->crt , dmg);
	add_enm_dmg(ply_ptr->name, cp->crt , dmg);
	
	cp->crt->hpcur -= dmg;
      }
      if(cp->crt->hpcur < 1) {
	print(fd, "\n당신의 활로 %M%j 죽였습니다.", cp->crt ,"3");
	broadcast_rom(fd, ply_ptr->rom_num,
		      "\n%M이 %s쪽에 있는 %M을 엄호하여 %m%j 죽였습니다.", 
		      ply_ptr, ext_ptr->name, crt_ptr, cp->crt,"3");
	broadcast_rom(fd, crt_ptr->rom_num,
		      "\n%M이 %M을 엄호하여 %m%j 죽였습니다.", 
		      ply_ptr, crt_ptr, cp->crt,"3");
	die(cp->crt, ply_ptr);
	cp = new_rom->first_mon;
      }
      else {
	check_for_flee(ply_ptr, cp->crt);
	cp = cp->next_tag;
      }
    }
  }
  else {
	 print(fd, "당신의 ");
	 ANSI(fd, YELLOW);
	 print(fd, "엄호");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "가 효과가 없습니다..\n");
    broadcast_rom(fd, ply_ptr->rom_num,
		  "\n%M이 %s쪽에 있는 %M을 엄호하였으나 실패했습니다.\n", ply_ptr, ext_ptr->name, crt_ptr);
  }
  ply_guard_time[fd] = t+ (15- MIN(10, ply_ptr->dexterity/3));  
  return(0);
}
/**********************************************************************/
/*                             사자후                               */
/**********************************************************************/
long ply_lion_scream_time[PMAX];
int lion_scream(ply_ptr, cmnd)
creature        *ply_ptr;
cmd            *cmnd;
{
   ctag            *cp;
   xtag            *xp;
   room            *rom_ptr;
   long            i, t;
   int              chance, j, k, dmg, fd, enm_thaco=0, m2=0;
   fd = ply_ptr->fd;
   
   rom_ptr = ply_ptr->parent_rom;
  
  if(ply_ptr->class < INVINCIBLE && !(ply_ptr->class == PALADIN && ply_ptr->level >= 50)) {

	 ANSI(fd, CYAN);
	 print(fd, "무사");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, " 레벨 ");
	 ANSI(fd, CYAN);
	 print(fd, "50");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이상만 쓸수 있는 기술입니다.\n");
	 return(0);

  }
  if(ply_ptr->class >= INVINCIBLE && !S_ISSET(ply_ptr, SPALADIN)) {

	 print(fd, "아직 ");
	 ANSI(fd, CYAN);
	 print(fd, "무사");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "를 ");
	 ANSI(fd, CYAN);
	 print(fd, "무적수련");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "하지 않았습니다.\n");
	 return(0);

  }
   
   t = time(0);
   
   if(ply_lion_scream_time[fd] > t) {
     please_wait(fd, ply_lion_scream_time[fd] -t);
     return(0);
   }
   cp = rom_ptr->first_mon;
   while(cp) {
     
     if((F_ISSET(ply_ptr, PDINVI) ? 1:!F_ISSET(cp->crt, MINVIS)) && !F_ISSET(cp->crt, MHIDDN) 
	&& !F_ISSET(cp->crt, MUNKIL )) {
       
       m2++;
       enm_thaco += (20 - cp->crt->thaco);
       add_enm_crt(ply_ptr->name, cp->crt);
       }
     cp = cp->next_tag;
   }

   if(m2<1) {
     print(fd,"이 방에는 당신이 공격할 적이 없습니다.");
     return(0);
   }
   ply_ptr->lasttime[LT_ATTCK].ltime = t;
   
   F_CLR(ply_ptr, PHIDDN);
   if(F_ISSET(ply_ptr, PINVIS)) {
      F_CLR(ply_ptr, PINVIS);
      print(fd, "당신의 모습이 서서히 드러납니다.\n");
      broadcast_rom(fd, ply_ptr->rom_num, "\n%M의 모습이 서서히 드러납니다.",
		    ply_ptr);
   }
   print(fd,"\n심후한 공력으로 부터 나온 사자후의 소리가 천지를 진동하니\n");
   print(fd,"어느누가 그 소리를 듣고서 성할수가 있으리오~\n");
   print(fd,"\"내 앞의 모든 적들은 귀에서 피를 흘리며 쓰러지리라~\"\n\n");
   print(fd,"당신은 뱃속에서부터 공력을 끌어올려 사자후를 내지릅니다.\n");
   chance = (20- ply_ptr->thaco) + 2*bonus[ply_ptr->dexterity] -  enm_thaco / m2 + (ply_ptr->level+29)/30;
   chance = MIN(chance, 20);
   if(chance < 5) chance = 5;
   if (mrand(1,22) <= chance) {
     k = MIN((chance+1)/3, m2);
     cp = rom_ptr->first_mon;
     for( j=0 ;j<k;j++)  {
       if((F_ISSET(ply_ptr, PDINVI) ? 1:!F_ISSET(cp->crt, MINVIS)) && !F_ISSET(cp->crt, MHIDDN) 
	  && !F_ISSET(cp->crt, MUNKIL )) {
	 dmg = (30- ply_ptr->thaco) + mrand(1, MIN(30,(20- ply_ptr->thaco)));
	 dmg = MIN(cp->crt->hpcur, dmg);
	 
	 if(mrand(0,1) && (ply_ptr->piety < cp->crt->piety) && F_ISSET(cp->crt, MMGONL)) {
	   print(fd, "당신의 사자후가 %M에게 아무런 상처도 내지 못합니다.\n", cp->crt);
	   dmg = 1;
	 }

		 print(fd, "당신은 ");
		 ANSI(fd, YELLOW);
		 print(fd, "사자후");
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "를 내질러 %m에게 ", cp->crt);
		 ANSI(fd, GREEN);
		 print(fd, "%d", dmg);
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "점의 피해를 입혔습니다.\n");

/*	 print(fd, "\n당신은 사자후를 내질러 %m에게 %d의 피해를 입힙니다.\n", cp->crt, dmg);  */

	 broadcast_rom(fd, ply_ptr->rom_num,
		       "\n%M이 사자후를 내질러 %m에게 %d의 피해를 입혔습니다.\n", 
		       ply_ptr,cp->crt , dmg);
	 add_enm_dmg(ply_ptr->name, cp->crt , dmg);
	 cp->crt->hpcur -= dmg;
       }
       
       if(cp->crt->hpcur < 1) {
	 print(fd, "\n당신의 뛰어난 공력으로 %M%j 죽였습니다.", cp->crt ,"3");
	 broadcast_rom(fd, ply_ptr->rom_num,
		       "\n%M%j 사자후로 %M%j 죽였습니다.", ply_ptr,"1", cp->crt,"3");
	 die(cp->crt, ply_ptr);
	 cp = rom_ptr->first_mon;
       }
       else {
	 check_for_flee(ply_ptr, cp->crt);
	 cp = cp->next_tag;
       }
     }
     xp = rom_ptr->first_ext;
     while(xp) {
       if(is_rom_loaded(xp->ext->room))
	 broadcast_rom(fd, xp->ext->room, "\n근처에 있는 %M의 사자후가 여기까지 울려퍼집니다.", ply_ptr);
       xp = xp->next_tag;
       
     }
   }
     
     
   else {
	 print(fd, "당신의 ");
	 ANSI(fd, YELLOW);
	 print(fd, "사자후");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "가 적의 공력을 이기지 못합니다.\n");
     broadcast_rom(fd, ply_ptr->rom_num,
		   "\n%M의 사자후가 적의 공력을 이기지 못합니다.\n", ply_ptr);
     ply_ptr->hpcur -=  ply_ptr->hpcur/10;
     print(fd, "당신은 약간 피로해짐을 느낍니다.");
   }
   ply_lion_scream_time[fd] = t+ (15 -MIN(7, ply_ptr->piety/4 + ply_ptr->intelligence/5));
   return(0);
}
/**********************************************************************/
/*                             변수나한권                               */
/**********************************************************************/
long ply_bnahan_time[PMAX];
int bnahan(ply_ptr, cmnd)
creature        *ply_ptr;
cmd            *cmnd;
{
   ctag            *cp;
   room            *rom_ptr;
   long            i, t;
   int              chance, j, k, dmg, fd, m2=0, crt_level=0;
   fd = ply_ptr->fd;
   
   rom_ptr = ply_ptr->parent_rom;
  
  if(ply_ptr->class < INVINCIBLE && !(ply_ptr->class == BARBARIAN && ply_ptr->level >= 50)) {

	 ANSI(fd, CYAN);
	 print(fd, "권법가");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, " 레벨 ");
	 ANSI(fd, CYAN);
	 print(fd, "50");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이상만 쓸수 있는 기술입니다.\n");
	 return(0);

  }
  if(ply_ptr->class >= INVINCIBLE && !S_ISSET(ply_ptr, SBARBARIAN)) {

	 print(fd, "아직 ");
	 ANSI(fd, CYAN);
	 print(fd, "권법가");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "을 ");
	 ANSI(fd, CYAN);
	 print(fd, "무적수련");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "하지 않았습니다.\n");
	 return(0);

  }
   
   t = time(0);
   
   if(ply_bnahan_time[fd] > t) {
     please_wait(fd, ply_bnahan_time[fd] -t);
     return(0);
   }
   cp = rom_ptr->first_mon;
   while(cp) {
     
     if((F_ISSET(ply_ptr, PDINVI) ? 1:!F_ISSET(cp->crt, MINVIS)) && !F_ISSET(cp->crt, MHIDDN) 
	&& !F_ISSET(cp->crt, MUNKIL )) {
       
       m2++;
       crt_level += cp->crt->level;
       add_enm_crt(ply_ptr->name, cp->crt);
       }
     cp = cp->next_tag;
   }

   if(m2<1) {
     print(fd,"이 방에는 당신이 공격할 적이 없습니다.");
     return(0);
   }
   ply_ptr->lasttime[LT_ATTCK].ltime = t;
   
   F_CLR(ply_ptr, PHIDDN);
   if(F_ISSET(ply_ptr, PINVIS)) {
      F_CLR(ply_ptr, PINVIS);
      print(fd, "당신의 모습이 서서히 드러납니다.\n");
      broadcast_rom(fd, ply_ptr->rom_num, "\n%M의 모습이 서서히 드러납니다.",
		    ply_ptr);
   }
   print(fd,"\n\"변수나한권~~!! 내 손을 보라.. 너의 어디를 공격할지 나도 모른다.\"\n");
   print(fd,"당신이 변수나한권을 시전하여 주먹의 위치가 이리저리 변화하던 중..\n");
   print(fd,"갑자기 소림나한권의 자세로 적을 공격하니..적이 어찌 피하랴..\n\n");
   print(fd,"당신의 손이 이리저리 왔다갔다 변하다가 적을 공격합니다.\n");
   
   chance = 50 + (((ply_ptr->level+3)/4) - ((crt_level/m2)/4)*2 +
     bonus[ply_ptr->strength]*5 + bonus[ply_ptr->dexterity]*7);
   chance = MIN(90, chance);
   if(F_ISSET(ply_ptr, PBLIND))
     chance = MIN(20, chance);
   
   if(mrand(1,100) <= chance) {
     k = MIN((chance+1)/3, m2);
     cp = rom_ptr->first_mon;
     for( j=0 ;j<k;j++)  {
       if((F_ISSET(ply_ptr, PDINVI) ? 1:!F_ISSET(cp->crt, MINVIS)) && !F_ISSET(cp->crt, MHIDDN) 
	  && !F_ISSET(cp->crt, MUNKIL )) {
	 dmg = mrand(1, MIN(30,(20- ply_ptr->thaco))) + mdice(ply_ptr)*4;
	 dmg = MIN(cp->crt->hpcur, dmg);
	 
	 if(mrand(0,1) && (ply_ptr->piety < cp->crt->piety) && F_ISSET(cp->crt, MMGONL)) {

	   print(fd, "당신의 변수나한권이 %M에게 아무런 상처도 내지 못합니다.\n", cp->crt);
	   dmg = 1;
	 }

		 print(fd, "당신은 ");
		 ANSI(fd, YELLOW);
		 print(fd, "변수나한권");
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "으로 %m에게 ", cp->crt);
		 ANSI(fd, GREEN);
		 print(fd, "%d", dmg);
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "점의 피해를 입혔습니다.\n");

/*	 print(fd, "\n당신은 변수나한권으로 %m에게 %d의 피해를 입힙니다.\n", cp->crt, dmg);  */
	 broadcast_rom(fd, ply_ptr->rom_num,
		       "\n%M이 변수나한권으로 %m에게 %d의 피해를 입혔습니다.\n", 
		       ply_ptr,cp->crt , dmg);
	 add_enm_dmg(ply_ptr->name, cp->crt , dmg);
	 cp->crt->hpcur -= dmg;
       }
       
       if(cp->crt->hpcur < 1) {
	 print(fd, "\n당신의 뛰어난 변수나한권으로 %M%j 죽였습니다.", cp->crt ,"3");
	 broadcast_rom(fd, ply_ptr->rom_num,
		       "\n%M%j 변수나한권으로 %M%j 죽였습니다.", ply_ptr,"1", cp->crt,"3");
	 die(cp->crt, ply_ptr);
	 cp = rom_ptr->first_mon;
       }
       else {
	 check_for_flee(ply_ptr, cp->crt);
	 cp = cp->next_tag;
       }
     }
   }
     
     
   else {
	 print(fd, "당신의 ");
	 ANSI(fd, YELLOW);
	 print(fd, "변수나한권");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이 적의 방어를 공략하지 못합니다.\n");
     broadcast_rom(fd, ply_ptr->rom_num,
		   "\n%M의 변수나한권이 적의 방어를 공략하지 못합니다.\n", ply_ptr);
     ply_ptr->hpcur -=  ply_ptr->hpcur/10;
     print(fd, "당신은 약간 피로해짐을 느낍니다.");
   }
   ply_bnahan_time[fd] = t+ (15 - MIN(7,ply_ptr->dexterity/4));
   return(0);
}
/**********************************************************************/
/*                             타구봉법                               */
/**********************************************************************/
long ply_tagu_time[PMAX];
int tagu(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
   creature        *crt_ptr;
   room            *rom_ptr;
   long            i, t;
   int             j, k=0, m=0, n, chance, chance2, fd, p, q, addprof;
   
   fd = ply_ptr->fd;
   rom_ptr = ply_ptr->parent_rom;
   
   if(cmnd->num < 2 || F_ISSET(ply_ptr, PBLIND)) {
      print(fd, "누굴 공격합니까?\n");
      return(0);
   }
   
   if(ply_ptr->class < INVINCIBLE && !(ply_ptr->class == THIEF && ply_ptr->level >= 50)) {
   
	 ANSI(fd, CYAN);
	 print(fd, "도둑");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, " 레벨 ");
	 ANSI(fd, CYAN);
	 print(fd, "50");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이상만 쓸수 있는 기술입니다.\n");
	 return(0);

   }
   if(ply_ptr->class >= INVINCIBLE && !S_ISSET(ply_ptr, STHIEF)) {

	 print(fd, "아직 ");
	 ANSI(fd, CYAN);
	 print(fd, "도둑");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "을 ");
	 ANSI(fd, CYAN);
	 print(fd, "무적수련");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "하지 않았습니다.\n");
	 return(0);

   }
   t = time(0);
   
   if(ply_tagu_time[fd] > t) {
      please_wait(fd, ply_tagu_time[fd] -t);
      return(0);
   }
   if(!ply_ptr->ready[WIELD-1] || (ply_ptr->ready[WIELD-1]->type != BLUNT )) {

	 ANSI(fd, YELLOW);
	 print(fd, "타구봉법");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "을 구사하시려면 ");
	 ANSI(fd, RED);
	 print(fd, "봉종류");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "의 무기가 필요합니다.");
	 return(0);

   }
   
   crt_ptr = find_crt(ply_ptr, rom_ptr->first_mon,
		      cmnd->str[1], cmnd->val[1]);
   
   if(!crt_ptr) {
      cmnd->str[1][0] = up(cmnd->str[1][0]);
      crt_ptr = find_crt(ply_ptr, rom_ptr->first_ply,
			 cmnd->str[1], cmnd->val[1]);
      
      if(!crt_ptr || crt_ptr == ply_ptr) {
	 print(fd, "그런 것은 여기 없습니다.\n");
	 return(0);
      }
      
   }
   
   if(crt_ptr->type == PLAYER) {
      if(!AT_WAR && F_ISSET(rom_ptr, RNOKIL)) {
	 print(fd, "이 방에서는 싸울 수 없습니다.\n");
	 return(0);
      }
      
      if(!F_ISSET(ply_ptr, PFAMIL) || !F_ISSET(crt_ptr, PFAMIL)) {
	 if(!F_ISSET(ply_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV)) {
	    print(fd, "당신은 선해서 다른 사용자를 공격할 수 없습니다.\n");
	    return (0);
	 }
	 if(!F_ISSET(crt_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV)) {
	    print(fd, "그 사용자는 선해서 보호받고 있습니다.\n");
	    return (0);
	 }
      }
      else if(check_war(ply_ptr->daily[DL_EXPND].max, crt_ptr->daily[DL_EXPND].max)) {
	 if(!F_ISSET(ply_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV)) {
	    print(fd, "당신은 선해서 다른 사용자를 공격할 수 없습니다.\n");
	    return (0);
	 }
	 if(!F_ISSET(crt_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV)) {
	    print(fd, "그 사용자는 선해서 보호받고 있습니다.\n");
	    return (0);
	 }
      }
      if(is_charm_crt(ply_ptr->name, crt_ptr)&& F_ISSET(crt_ptr, PCHARM)) {
	 print(fd, "당신은 %S%j 너무 좋아해 그렇게 할 수 없습니다.\n", crt_ptr->name,"3");
	 return(0);
      }
      
   }
   ply_ptr->lasttime[LT_ATTCK].ltime = t;
   
   F_CLR(ply_ptr, PHIDDN);
   if(F_ISSET(ply_ptr, PINVIS)) {
      F_CLR(ply_ptr, PINVIS);
      print(fd, "당신의 모습이 서서히 드러납니다.\n");
      broadcast_rom(fd, ply_ptr->rom_num, "\n%M의 모습이 서서히 드러납니다.",
		    ply_ptr);
   }
   
   if(crt_ptr->type != PLAYER) {
      if(F_ISSET(crt_ptr, MUNKIL)) {
	 print(fd, "당신은 %s를 해칠 수 없습니다.\n",
	       F_ISSET(crt_ptr, MMALES) ? "그":"그녀");
	 return(0);
      }
      if(mrand(0,1) && (ply_ptr->piety < crt_ptr->piety) && F_ISSET(crt_ptr, MMGONL)) {
		  print(fd, "당신의 공격이 %M에게 아무소용이 없는듯 합니다.\n", crt_ptr);
	 return(0);
      }
      if(mrand(0,1) && F_ISSET(crt_ptr, MENONL)) {
	 if(!ply_ptr->ready[WIELD-1] ||
	    ply_ptr->ready[WIELD-1]->adjustment < 1) {
	    print(fd, "당신의 공격이 %M에게 아무 소용이 없는듯 합니다.\n", crt_ptr);
	    return(0);
	 }
      }
      add_enm_crt(ply_ptr->name, crt_ptr);
   }
   print(fd, "\n\"개방의 방주 황용 여협이시여. 지금 내 앞에 나타가 저를 도와 주소서~\"\n");
   print(fd, "당신이 외치자 황용이 나타가 당신을 돕습니다.\n\n");
   
   chance = 50 + (((ply_ptr->level+3)/4) - ((crt_ptr->level+3)/4))*2 +  bonus[ply_ptr->intelligence]*2 +
     bonus[ply_ptr->dexterity]*7;
   chance = MIN(90, chance);
   if(F_ISSET(ply_ptr, PBLIND))
     chance = MIN(20, chance);
   
   if(mrand(1,100) <= chance) {
     if(ply_ptr->ready[WIELD-1]->shotscur > 0)
       ply_ptr->ready[WIELD-1]->shotscur--;
     
     if(ply_ptr->ready[WIELD-1]) {
       if(ply_ptr->ready[WIELD-1]->shotscur < 1) {
	 print(fd, "\n%S%j 부서져 버렸습니다.\n",
	       ply_ptr->ready[WIELD-1]->name,"1");
	 add_obj_crt(ply_ptr->ready[WIELD-1], ply_ptr);
	 ply_ptr->ready[WIELD-1] = 0;
	 return(0);
       }
     }
     
     n = ply_ptr->thaco - crt_ptr->armor/10;
     if(mrand(1,20) >= n) {
       chance2 = (20-ply_ptr->thaco)/10 + mrand(1,((ply_ptr->level+29)/30));
       print(fd, "당신은 타구봉법을 시전해 적을 개를 패듯이 물리칩니다. 깨갱~\n\n");
       ply_tagu_time[fd] = t+ (20-ply_ptr->dexterity/7);  
       for(j=0; j < chance2; j++){ 
	 n = mdice(ply_ptr) + mdice(ply_ptr->ready[WIELD-1])*3;
	 m = MIN(crt_ptr->hpcur, n);     
	 if(ply_ptr->ready[WIELD-1]) {
/*	   q = MIN(ply_ptr->ready[WIELD-1]->type, MISSILE);
	   addprof = (m * crt_ptr->experience) / (crt_ptr->hpmax * chance2) ;
	   addprof = MIN(addprof, crt_ptr->experience);
	   ply_ptr->proficiency[q] += addprof;
버그로 인해서 prof증가 시키는 곳 삭제.*/
	 }
	 
	 /* crt_ptr->hpcur -= n; */

		 print(fd, "당신은 ");
		 ANSI(fd, YELLOW);
		 print(fd, "타구봉법");
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "으로 %m에게 ", crt_ptr);
		 ANSI(fd, GREEN);
		 print(fd, "%d", m);
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "점의 피해를 입혔습니다.\n");
	 
/*	 print(fd, "당신은 타구봉법으로 %d점의 공격을 가했습니다.\n\n", m);  */
	    broadcast_rom2(fd, crt_ptr->fd, crt_ptr->rom_num,
			   "\n\n%M이 %M에게 타구봉법으로 %d점의 공격을 가합니다.", ply_ptr, crt_ptr, m);
	    print(crt_ptr->fd, "%M이 타구봉법으로 %d점의 공격을 가했습니다.\n\n", ply_ptr, m);
	    crt_ptr->hpcur -= m;
	    k += m;
	    if(crt_ptr->hpcur < 1) break;
	 }
	 if(crt_ptr->type != PLAYER) {
                                add_enm_dmg(ply_ptr->name, crt_ptr, k);
	 }
	print(fd, "당신은 총"); 
	ANSI(fd, GREEN);
	print(fd," %d 연타",j);
	ANSI(fd, YELLOW);
	print(fd, " %d점",k);
	ANSI(fd, WHITE);
	print(fd, "의 공격을");
	ANSI(fd, CYAN);
	print(fd, " %M",crt_ptr);
	ANSI(fd, WHITE);
	print(fd, "에게 가했습니다.\n");	 
	 if(crt_ptr->hpcur < 1) {
	    print(fd, "당신은 %M%j 죽였습니다.\n", crt_ptr,"3");
	    broadcast_rom2(fd, crt_ptr->fd,
			   ply_ptr->rom_num,
                                               "\n%M%j %M%j 죽였습니다.", ply_ptr,"1",
			   crt_ptr,"3");
	    
	    die(crt_ptr, ply_ptr);
	 }
	 else
	   check_for_flee(ply_ptr, crt_ptr);
      }
      
      
      else {
	 print(fd, "당신의 ");
	 ANSI(fd, YELLOW);
	 print(fd, "타구봉법");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이 실패했습니다.\n");

	 print(crt_ptr->fd, "\n%M이 당신에게 타구봉법을 구사하려고 합니다.\n", ply_ptr);
	 broadcast_rom2(fd, crt_ptr->fd, crt_ptr->rom_num,
			"\n%M이 %M에게 타구봉법으로 공격하려고 합니다.", ply_ptr,
			crt_ptr);
	 ply_tagu_time[fd] = t+(15-ply_ptr->dexterity/6);
      }
   }
   
   else {
	 print(fd, "당신의 ");
	 ANSI(fd, YELLOW);
	 print(fd, "타구봉법");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이 실패했습니다.\n");
      print(crt_ptr->fd, "%M이 당신에게 타구봉법을 구사하려고 합니다.\n", ply_ptr);
      broadcast_rom2(fd, crt_ptr->fd, crt_ptr->rom_num,
		     "\n%M이 %M에게 타구봉법으로 공격하려고 합니다.", ply_ptr, crt_ptr);
      ply_tagu_time[fd] = t+ (15 -MIN(7, ply_ptr->dexterity/3));
   }
   return(0);
}

/**********************************************************************/
/*                              반탄강기 			       */
/**********************************************************************/

int reflect(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
  long    i, t;
  int     chance, fd;
  
  fd = ply_ptr->fd;
  
  if(ply_ptr->class < INVINCIBLE && !(ply_ptr->class == FIGHTER && ply_ptr->level >= 50)) {

	 ANSI(fd, CYAN);
	 print(fd, "검사");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, " 레벨 ");
	 ANSI(fd, CYAN);
	 print(fd, "50");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이상만 쓸수 있는 기술입니다.\n");
	 return(0);

  }
  if(ply_ptr->class >= INVINCIBLE && !S_ISSET(ply_ptr, SFIGHTER)) {

	 print(fd, "아직 ");
	 ANSI(fd, CYAN);
	 print(fd, "검사");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "을 ");
	 ANSI(fd, CYAN);
	 print(fd, "무적수련");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "하지 않았습니다.\n");
	 return(0);

  }
  
  if(F_ISSET(ply_ptr, PREFLECT)) {
    print(fd, "당신은 지금 반탄강기를 사용중입니다.\n");
    return(0);
  }
  
  i = ply_ptr->lasttime[LT_REFLECT].ltime;
  t = time(0);
  
  if(t-i < 600L) {
    print(fd, "%d분 %02d초 기다리세요.\n",
	  (600L-t+i)/60L, (600L-t+i)%60L);
    return(0);
  }
  
  chance = MIN(20, ((ply_ptr->level+3)/50)*2 + bonus[20-ply_ptr->thaco]);
  
  if(mrand(1,100) <= chance) {
    print(fd, "\n주변의 살기를 느끼고 십이성 공력을 끓어올려 몸을 보호하니");
    print(fd, "\n무엇이 이 반탄강기를 뚫으리요~");
    broadcast_rom(fd, ply_ptr->rom_num, "%M이 공력을 끌어올려 몸 주위에 반탄강기를 형성합니다.", ply_ptr);
    F_SET(ply_ptr, PREFLECT);
    ply_ptr->lasttime[LT_REFLECT].ltime = t;
    ply_ptr->lasttime[LT_REFLECT].interval = 500L;
    
  }
  else {
	 ANSI(fd, YELLOW);
	 print(fd, "반탄강기");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "를 형성하는데 실패했습니다.\n");
    broadcast_rom(fd, ply_ptr->rom_num, "%M이 반탄강기를 시도합니다.",
		  ply_ptr);
    ply_ptr->lasttime[LT_REFLECT].ltime = t - 300L;
  }
  
  return(0);
}

/*
 *
 * COMMAND14.C
 *
 * Copyright (C) 1998 Donghyun Kim
 *
 * Add user routine...
 */

/**********************************************************************/
/*                        분신술 - 자객용                             */
/**********************************************************************/

long ply_shadow_time[PMAX];

int shadow(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
   creature        *crt_ptr;
   room            *rom_ptr;
   long            i, t;
   int             j, k=0, m=0, n, chance, chance2, fd, p, q, addprof;

   fd = ply_ptr->fd;
   rom_ptr = ply_ptr->parent_rom;

   if(cmnd->num < 2 || F_ISSET(ply_ptr, PBLIND))
   {
	  print(fd, "누굴 공격합니까?\n");
	  return(0);
   }

   if(ply_ptr->class < INVINCIBLE && !(ply_ptr->class == ASSASSIN && ply_ptr->level >= 50))
   {
	 ANSI(fd, CYAN);
	 print(fd, "자객");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, " 레벨 ");
	 ANSI(fd, CYAN);
	 print(fd, "50");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이상만 쓸수 있는 기술입니다.\n");
	 return(0);
   }

   if(ply_ptr->class >= INVINCIBLE && !S_ISSET(ply_ptr, SASSASSIN))
   {
	 print(fd, "아직 ");
	 ANSI(fd, CYAN);
	 print(fd, "자객");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "을 ");
	 ANSI(fd, CYAN);
	 print(fd, "무적수련");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "하지 않았습니다.\n");
	 return(0);
   }

   t = time(0);

   if(ply_shadow_time[fd] > t)
   {
	  please_wait(fd, ply_shadow_time[fd] -t);
	  return(0);
   }

   if(!ply_ptr->ready[WIELD-1] || (ply_ptr->ready[WIELD-1]->type != SHARP ))
   {
	 ANSI(fd, YELLOW);
	 print(fd, "분신술");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "을 구사하시려면 ");
	 ANSI(fd, RED);
	 print(fd, "도종류");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "의 무기가 필요합니다.");
	 return(0);
   }


   crt_ptr = find_crt(ply_ptr, rom_ptr->first_mon,
			  cmnd->str[1], cmnd->val[1]);

   if(!crt_ptr)
   {
	  cmnd->str[1][0] = up(cmnd->str[1][0]);
	  crt_ptr = find_crt(ply_ptr, rom_ptr->first_ply,
			 cmnd->str[1], cmnd->val[1]);

	  if(!crt_ptr || crt_ptr == ply_ptr)
	  {
	 print(fd, "그런 것은 여기 없습니다.\n");
	 return(0);
	  }

   }

   if(crt_ptr->type == PLAYER)
   {
	  if(!AT_WAR && F_ISSET(rom_ptr, RNOKIL))
	  {
	 print(fd, "이 방에서는 싸울 수 없습니다.\n");
	 return(0);
	  }

	  if(!F_ISSET(ply_ptr, PFAMIL) || !F_ISSET(crt_ptr, PFAMIL))
	  {
	 if(!F_ISSET(ply_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV))
		 {
		print(fd, "당신은 선해서 다른 사용자를 공격할 수 없습니다.\n");
		return (0);
	 }
	 if(!F_ISSET(crt_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV))
		 {
		print(fd, "그 사용자는 선해서 보호받고 있습니다.\n");
		return (0);
	 }
	  }
	  else if(check_war(ply_ptr->daily[DL_EXPND].max, crt_ptr->daily[DL_EXPND].max))
	  {
	 if(!F_ISSET(ply_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV))
		 {
		print(fd, "당신은 선해서 다른 사용자를 공격할 수 없습니다.\n");
		return (0);
	 }
	 if(!F_ISSET(crt_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV))
		 {
		print(fd, "그 사용자는 선해서 보호받고 있습니다.\n");
		return (0);
	 }
	  }
	  if(is_charm_crt(ply_ptr->name, crt_ptr)&& F_ISSET(crt_ptr, PCHARM))
	  {
	 print(fd, "당신은 %S%j 너무 좋아해 그렇게 할 수 없습니다.\n", crt_ptr->name,"3");
	 return(0);
	  }

   }

   ply_ptr->lasttime[LT_ATTCK].ltime = t;

   F_CLR(ply_ptr, PHIDDN);
   if(F_ISSET(ply_ptr, PINVIS))
   {
	  F_CLR(ply_ptr, PINVIS);
	  print(fd, "당신의 모습이 서서히 드러납니다.\n");
	  broadcast_rom(fd, ply_ptr->rom_num, "\n%M의 모습이 서서히 드러납니다.",
			ply_ptr);
   }

   if(crt_ptr->type != PLAYER)
   {
	  if(F_ISSET(crt_ptr, MUNKIL))
	  {
	 print(fd, "당신은 %s를 해칠 수 없습니다.\n",
		   F_ISSET(crt_ptr, MMALES) ? "그":"그녀");
	 return(0);
	  }
	  if(mrand(0,1) && (ply_ptr->piety < crt_ptr->piety) && F_ISSET(crt_ptr, MMGONL))
	  {
	 print(fd, "당신의 공격이 %M에게 아무소용이 없는듯 합니다.\n", crt_ptr);
	 return(0);
	  }
	  if(mrand(0,1) && F_ISSET(crt_ptr, MENONL))
	  {
	 if(!ply_ptr->ready[WIELD-1] ||
		ply_ptr->ready[WIELD-1]->adjustment < 1)
		 {
		print(fd, "당신의 공격이 %M에게 아무 소용이 없는듯 합니다.\n", crt_ptr);
		return(0);
	 }
	  }
	  add_enm_crt(ply_ptr->name, crt_ptr);
   }

   print(fd, "\n\"이것은 동방 최고의 닌자 기술이니~ 나를 구별할 자 아무도 없으리라!\"\n");
   print(fd, "당신은 두 손을 합장하며 온 몸에 기를 모읍니다.\n\n");

   chance = 50 + (((ply_ptr->level+3)/4) - ((crt_ptr->level+3)/4))*2 +  bonus[ply_ptr->intelligence]*2 +
	 bonus[ply_ptr->dexterity]*7;

   chance = MIN(90, chance);
   if(F_ISSET(ply_ptr, PBLIND))
	 chance = MIN(20, chance);

   if(mrand(1,100) <= chance)
   {

	 if(ply_ptr->ready[WIELD-1]->shotscur > 0)
	   ply_ptr->ready[WIELD-1]->shotscur--;

	 if(ply_ptr->ready[WIELD-1])
	 {
	   if(ply_ptr->ready[WIELD-1]->shotscur < 1)
	   {
	 print(fd, "\n%S%j 부서져 버렸습니다.\n",
		   ply_ptr->ready[WIELD-1]->name,"1");
	 add_obj_crt(ply_ptr->ready[WIELD-1], ply_ptr);
	 ply_ptr->ready[WIELD-1] = 0;
	 return(0);
	   }
	 }

	 n = ply_ptr->thaco - crt_ptr->armor/10;
	 if(mrand(1,20) >= n)
	 {
	   if (ply_ptr->class < INVINCIBLE) chance2 = NOR_SHADOW;
	   if (ply_ptr->class == INVINCIBLE && S_ISSET(ply_ptr, SASSASSIN))
										chance2 = INV_SHADOW;
	   if (ply_ptr->class > INVINCIBLE) chance2 = CAR_SHADOW;
		if(S_ISSET(ply_ptr, YELLOWI)) chance2 += 1;
		if(ply_ptr->class == BULSA) chance2 += 1;
	   print(fd, "동방 최고의 기술 분신술...\n");
		print(fd, "\n당신의 분신 %d명이 적을 공격합니다...\n\n",chance2);

	   ply_shadow_time[fd] = t + (8 - MIN(3, ply_ptr->dexterity/10));


	for(j=0; j < chance2; j++)
		{

	 n = (mdice(ply_ptr)*mrand(1,2) + mdice(ply_ptr->ready[WIELD-1])*mrand(1,2) + (ply_ptr->level - 
50)/10 )/2;
	 m = MIN(crt_ptr->hpcur, n);

	 if(ply_ptr->ready[WIELD-1])
		 {
/*	   q = MIN(ply_ptr->ready[WIELD-1]->type, MISSILE);
	   addprof = (m * crt_ptr->experience) / (crt_ptr->hpmax * chance2) ;
	   addprof = MIN(addprof, crt_ptr->experience);
	   ply_ptr->proficiency[q] += addprof;
버그로 중지.*/
	 }

	 /* crt_ptr->hpcur -= n; */
	 	print(fd, "당신의");
		ANSI(fd, CYAN);
		print(fd, " %2d번째 분신이", j+1);
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, " %M에게 ", crt_ptr);
		 ANSI(fd, GREEN);
		 print(fd, "%d", m);
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "점의 피해를 입혔습니다.\n");
		broadcast_rom2(fd, crt_ptr->fd, crt_ptr->rom_num,
			   "\n%M이 %M에게 분신술로 %d점의 피해를 입혔습니다.", ply_ptr, crt_ptr, m);
		print(crt_ptr->fd, "%M이 분신술로 %d점의 공격을 가했습니다.\n", ply_ptr, m);
		crt_ptr->hpcur -= m;
		k += m;
		if(crt_ptr->hpcur < 1) break;
	}
	 if(crt_ptr->type != PLAYER)
		 {
		 add_enm_dmg(ply_ptr->name, crt_ptr, k);
	 }

	 if(crt_ptr->hpcur < 1)
		 {
		print(fd, "당신의 분신들이 %M%j 죽였습니다.\n", crt_ptr,"3");
		broadcast_rom2(fd, crt_ptr->fd,
			   ply_ptr->rom_num, "\n\n%M%j %M%j 죽였습니다.", ply_ptr,"1",
			   crt_ptr,"3");

		die(crt_ptr, ply_ptr);
	 }
	 else
	   check_for_flee(ply_ptr, crt_ptr);
	  }


	  else
	  {
	 print(fd, "당신의 ");
		 ANSI(fd, YELLOW);
		 print(fd, "분신술");
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "이 실패했습니다.\n");
	 print(crt_ptr->fd, "\n%M이 당신에게 분신술을 구사하려고 합니다.\n", ply_ptr);
	 broadcast_rom2(fd, crt_ptr->fd, crt_ptr->rom_num,
			"\n%M이 %M에게 분신술로 공격하려고 합니다.", ply_ptr,
			crt_ptr);
	 ply_shadow_time[fd] = t + (8 - MIN(5, ply_ptr->dexterity/5));
	  }
   }

   else
   {
	  print(fd, "당신의 ");
	  ANSI(fd, YELLOW);
	  print(fd, "분신술");
	  ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	  print(fd, "이 실패했습니다.\n");
	  print(crt_ptr->fd, "%M이 당신에게 분신술을 구사하려고 합니다.\n", ply_ptr);
	  broadcast_rom2(fd, crt_ptr->fd, crt_ptr->rom_num,
			 "\n%M이 %M에게 분신술로 공격하려고 합니다.", ply_ptr, crt_ptr);
	  ply_shadow_time[fd] = t + (8 - MIN(5, ply_ptr->dexterity/5));
   }
   return(0);

}













