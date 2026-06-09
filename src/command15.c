/*
 * COMMAND15.C:
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
#include <sys/types.h>
#include <sys/stat.h>
#include <time.h>


long sasal_time[PMAX];

int sasal(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
   creature        *crt_ptr;
   room            *rom_ptr;
   long            i, t;
   int             j, k=0, m=0, n, chance, chance2, fd, p;
   
   fd = ply_ptr->fd;
   rom_ptr = ply_ptr->parent_rom;
   
   if(cmnd->num < 2 || F_ISSET(ply_ptr, PBLIND)) {
      print(fd, "누굴 공격합니까?\n");
      return(0);
   }
   
   if(!S_ISSET(ply_ptr, YELLOWI)) {

	 ANSI(fd, CYAN);
	 print(fd, "노랑초인");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이상만 쓸수 있는 기술입니다.\n");
	 return(0);

   }
   t = time(0);
   
   if(sasal_time[fd] > t) {
      please_wait(fd, sasal_time[fd] -t);
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
      else if(check_war(ply_ptr->daily[DL_EXPND].max,
crt_ptr->daily[DL_EXPND].max)) {
	 if(!F_ISSET(ply_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV)) {
	    print(fd, "당신은 선해서 다른 사용자를 공격할 수 없습니다.\n");
	    return (0);
	 }
	 if(!F_ISSET(crt_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV)) {
	    print(fd, "그 사용자는 선해서 보호받고 있습니다.\n");
	    return (0);
	 }
      }
      if(is_charm_crt(ply_ptr->name, crt_ptr)&& F_ISSET(crt_ptr, PCHARM))
{
	 print(fd, "당신은 %S%j 너무 좋아해 그렇게 할 수 없습니다.\n",
crt_ptr->name,"3");
	 return(0);
      }
      
   }
   if(!ply_ptr->ready[WIELD-1] || (ply_ptr->ready[WIELD-1]->type!=
MISSILE)) {
      print(fd, "확인사살을 구사하시려면 궁 종류의 무기가 필요합니다.");
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
   
   if(crt_ptr->type != PLAYER) {
      if(F_ISSET(crt_ptr, MUNKIL)) {
	 print(fd, "당신은 %s를 해칠 수 없습니다.\n",
	       F_ISSET(crt_ptr, MMALES) ? "그":"그녀");
	 return(0);
      }
      if(mrand(0,1) && (ply_ptr->piety < crt_ptr->piety) &&
F_ISSET(crt_ptr, MMGONL)) {
		  print(fd, "당신의 공격이 %M에게 아무소용이 없는듯 합니다.\n", crt_ptr);
	 return(0);
      }
      if(mrand(0,1) && F_ISSET(crt_ptr, MENONL)) {
	 if(!ply_ptr->ready[WIELD-1] ||
	    ply_ptr->ready[WIELD-1]->adjustment < 1) {
	    print(fd, "당신의 공격이 %M에게 아무 소용이 없는듯 합니다.\n",
crt_ptr);
	    return(0);
	 }
      }
      add_enm_crt(ply_ptr->name, crt_ptr);
   }

   print(fd, "\n황씨 가문의 특급기술.. 여기 확인사살이 있나니\n");
   print(fd, "황씨가문의 대가 황충님이시어 나에게 힘을 주소서\n");

	   chance = 50 + (((ply_ptr->level+3)/4) -
((crt_ptr->level+3)/4))*2 +  bonus[ply_ptr->intelligence]*2 +
	 bonus[ply_ptr->dexterity]*7;


   chance = MIN(90, chance);
   if(F_ISSET(ply_ptr, PBLIND))
     chance = MIN(20, chance);
   
   if(mrand(1,100) <= chance) {
      
      n = ply_ptr->thaco - crt_ptr->armor/10;
      if(mrand(1,20) >= n) {
	 chance2 = (20-ply_ptr->thaco)/9 +
mrand(1,((ply_ptr->level+29)/30));
	 
	sasal_time[fd] = t+ (30-ply_ptr->dexterity/7);  
	if(crt_ptr->hpmax/3 >= crt_ptr->hpcur) {
		print(fd,"\n당신은 완벽한 확인사살로 %M에게 %d점의 공격을 가했습니다.\n",crt_ptr->name, crt_ptr->hpcur);
	    broadcast_rom2(fd, crt_ptr->fd, crt_ptr->rom_num,
			   "\n\n%M이 %M에게 완벽한 확인사살로 %d점의 공격을 가합니다.", ply_ptr, crt_ptr, crt_ptr->hpcur);
	    print(crt_ptr->fd, "%M이 완벽한 확인사살로 %d점의 공격을 가했습니다.\n\n", ply_ptr, crt_ptr->hpcur);
	    
	 if(crt_ptr->type != PLAYER) {
         add_enm_dmg(ply_ptr->name, crt_ptr, crt_ptr->hpcur);
	 }
	  
	    print(fd, "당신은 %M%j 죽였습니다.\n", crt_ptr,"3");
	    broadcast_rom2(fd, crt_ptr->fd,
			   ply_ptr->rom_num,"\n%M%j %M%j 죽였습니다.",
ply_ptr,"1",
			   crt_ptr,"3");
	    
	    die(crt_ptr, ply_ptr);
	 
	}
	 else {
		n = mdice(ply_ptr) * 7 + mrand(0,ply_ptr->strength)*2 ;
		m=MIN(crt_ptr->hpcur, n);
		print(fd,"\n당신은 어색한 확인사살로 %M에게 %d점의 공격을 가했습니다.\n",crt_ptr->name, m);
	    broadcast_rom2(fd, crt_ptr->fd, crt_ptr->rom_num,
			   "\n\n%M이 %M에게 어색한 확인사살로 %d점의 공격을 가합니다.", ply_ptr, crt_ptr, m);
	    print(crt_ptr->fd, "%M이 어색한 확인사살로 %d점의 공격을 가했습니다.\n\n", ply_ptr, m);
	    crt_ptr->hpcur -= m;

	 if(crt_ptr->type != PLAYER) {
         add_enm_dmg(ply_ptr->name, crt_ptr, m);
	 }
	 
	 if(crt_ptr->hpcur < 1) {
	    print(fd, "당신은 %M%j 죽였습니다.\n", crt_ptr,"3");
	    broadcast_rom2(fd, crt_ptr->fd,
			   ply_ptr->rom_num,"\n%M%j %M%j 죽였습니다.",
ply_ptr,"1",
			   crt_ptr,"3");
	    
	    die(crt_ptr, ply_ptr);
	 }
	 else
	   check_for_flee(ply_ptr, crt_ptr);
      }
      
	}      
      else {
	 print(fd, "당신의 ");
	 ANSI(fd, YELLOW);
	 print(fd, "확인사살");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이 실패했습니다.\n");

	 print(crt_ptr->fd, "\n%M이 당신에게 확인사살을 구사하려고 합니다.\n", ply_ptr);
	 broadcast_rom2(fd, crt_ptr->fd, crt_ptr->rom_num,
			"\n%M이 %M에게 확인사살로 공격하려고 합니다.",
ply_ptr,
			crt_ptr);
	 sasal_time[fd] = t+(20-ply_ptr->dexterity/6);
      }
   }
   
   else {

	 print(fd, "당신의 ");
	 ANSI(fd, YELLOW);
	 print(fd, "확인사살");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이 실패했습니다.\n");

      print(crt_ptr->fd, "%M이 당신에게 확인사살을 구사하려고 합니다.\n",
ply_ptr);
      broadcast_rom2(fd, crt_ptr->fd, crt_ptr->rom_num,
		     "\n%M이 %M에게 확인사살로 공격하려고 합니다.",
ply_ptr, crt_ptr);
      sasal_time[fd] = t+ (20 -MIN(7, ply_ptr->dexterity/3));
   }
   return(0);

}

/**********************************************************************/
/*                             창격술                               */
/**********************************************************************/

long ply_chang_time[PMAX];

int chang(ply_ptr, cmnd)
creature        *ply_ptr;
cmd            *cmnd;
{
   ctag            *cp;
   room            *rom_ptr;
   long            i, t;
   int              chance, j, k, m, dmg, fd, enm_thaco=0, m2=0, dur, p,
addprof;
   fd = ply_ptr->fd;
   
   rom_ptr = ply_ptr->parent_rom;
  
   if(ply_ptr->class < CARETAKER) {
	 print(fd, "초인 이상만 쓸수 있는 기술입니다.\n");
	 return(0);
   }
  
   t = time(0);
   
   if(ply_chang_time[fd] > t) {
     please_wait(fd, ply_chang_time[fd] -t);
     return(0);
   }

   if(!ply_ptr->ready[WIELD-1] || (ply_ptr->ready[WIELD-1]->type != POLE))
{
     print(fd, "창격술을 구사하시려면 창 종류의 무기가 필요합니다.");
     return(0);
   }
   cp = rom_ptr->first_mon;
   while(cp) {
     
     if((F_ISSET(ply_ptr, PDINVI) ? 1:!F_ISSET(cp->crt, MINVIS)) &&
!F_ISSET(cp->crt, MHIDDN) \
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
      broadcast_rom(fd, ply_ptr->rom_num, "\n%M의 모습이 서서히 드러납니다.", ply_ptr);
   }

   print(fd,"\n창격술이란 길고 긴 창으로 적의 사지를 막는다.. \n");
   print(fd,"\"지금 내가 모여주는 이 기술은... 그 창격술이니.. ");
   print(fd,"    잘보아라\" \n\n당신은 창격술로 적의 사지를 동시에 공격합니다..\n\n");
   chance = (20- ply_ptr->thaco) + 2*bonus[ply_ptr->dexterity] -
enm_thaco / m2 + (ply_ptr->level+29)/30 ;
   chance = MIN(chance, 20);
   if(chance < 5) chance = 5;

   if (mrand(1,22) <= chance) {

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
     
     k = MIN((chance+1)/3, m2);
     cp = rom_ptr->first_mon;
     for( j=0 ;j<k;j++)  {
       if((F_ISSET(ply_ptr, PDINVI) ? 1:!F_ISSET(cp->crt, MINVIS)) &&
!F_ISSET(cp->crt, MHIDDN) \
	  && !F_ISSET(cp->crt, MUNKIL )) {
	 dmg = (mdice(ply_ptr)*2 +
mdice(ply_ptr->ready[WIELD-1])*4)*mrand(1,3);
	 dmg = MIN(cp->crt->hpcur, dmg);
	 
	 if(ply_ptr->ready[WIELD-1]) {
	   p = MIN(ply_ptr->ready[WIELD-1]->type, MISSILE);
	   addprof = (dmg * cp->crt->experience) / (cp->crt->hpmax * k);
	   addprof = MIN(addprof, cp->crt->experience);
	   ply_ptr->proficiency[p] += addprof;
	 }

	 if(mrand(0,1) && (ply_ptr->piety < cp->crt->piety) &&
F_ISSET(cp->crt, MMGONL)) {
	   print(fd, "당신의 창은 %M에게 아무런 상처도 내지 못합니다.\n",
cp->crt);
	   dmg = 1;
	 }
	 if(F_ISSET(cp->crt, MENONL) &&
ply_ptr->ready[WIELD-1]->adjustment < 1) {
	   print(fd, "당신의 창은 %M의 갑옷을 뚫기엔 역부족입니다.\n",
cp->crt);
	   dmg = 1;
	 }
	 add_enm_dmg(ply_ptr->name, cp->crt , dmg);
	 dur = MIN(20, dmg/20);
	 
	 		 print(fd, "당신은 ");
		 ANSI(fd, YELLOW);
		 print(fd, "창격술");
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "로 %m에게 " , cp->crt);
		 ANSI(fd, GREEN);
		 print(fd, "%d", dmg);
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "의 피해를 입힙니다.\n\n");

/*	 print(fd, "\n당신은 창격술으로 %m에게 %d의 피해를 입힙니다.\n",
cp->crt, dmg);     */
	 broadcast_rom(fd, ply_ptr->rom_num,
		       "\n%M이 창격술으로 %m에게 %d의 피해를 입혔습니다.\n", \
		       ply_ptr,cp->crt , dmg);
	 if(dur > 5) {
	 }
	 cp->crt->hpcur -= dmg;
       }	 
       if(cp->crt->hpcur < 1) {
	 print(fd, "\n당신의 매서운 창격술으로 %M%j 죽였습니다.", cp->crt
,"3");
	 broadcast_rom(fd, ply_ptr->rom_num,
		       "\n%M%j 창격술으로 %M%j 죽였습니다.", ply_ptr,"1",
cp->crt,"3");
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
	 print(fd, "창격술");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이 적의 기세에 눌려 실패했습니다.\n");

     broadcast_rom(fd, ply_ptr->rom_num,
		   "\n%M의 창격술이 적의 기세에 눌려 실패했습니다.\n",
ply_ptr);
     
     if( !F_ISSET(ply_ptr->ready[WIELD-1], ONSHAT) && \
	 ply_ptr->ready[WIELD-1]->shotscur <
ply_ptr->ready[WIELD-1]->shotsmax/2 && ((mrand(1,5) <= 2))) {
       print(fd, "\n%S%j 산산히 부서집니다.",
	     ply_ptr->ready[WIELD-1]->name,"1");
       broadcast_rom(fd, ply_ptr->rom_num,
		     "\n%s의 %S%j 산산히 부서집니다.",
		     F_ISSET(ply_ptr, PMALES) ? "그":"그녀",
		     ply_ptr->ready[WIELD-1]->name,"1");
       free_obj(ply_ptr->ready[WIELD-1]);
       ply_ptr->ready[WIELD-1] = 0;
     }
     
   }
   ply_chang_time[fd] = t+ (18 -MIN(7, ply_ptr->dexterity/4));
   return(0);
}



/**********************************************************************/
/*                             최루탄                               */
/**********************************************************************/

long ply_choi_time[PMAX];

int choi(ply_ptr, cmnd)
creature        *ply_ptr;
cmd            *cmnd;
{
   ctag            *cp;
   room            *rom_ptr;
   long            i, t;
   int              chance, j, k, m, dmg, fd, enm_thaco=0, m2=0, dur, p,
addprof;
   fd = ply_ptr->fd;
   
   rom_ptr = ply_ptr->parent_rom;

  if(ply_ptr->class < INVINCIBLE && !(ply_ptr->class == RANGER &&
ply_ptr->level >= 50)) {

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
  
   t = time(0);
   if(ply_choi_time[fd] > t) {
     please_wait(fd, ply_choi_time[fd] -t);
     return(0);
   }

  if(!ply_ptr->ready[WIELD-1] || (ply_ptr->ready[WIELD-1]->type != MISSILE
)) {

	 ANSI(fd, YELLOW);
	 print(fd, "최루탄");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "을 구사하시려면 ");
	 ANSI(fd, RED);
	 print(fd, "활종류");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "의 무기가 필요합니다.");
	 return(0);

  }
   cp = rom_ptr->first_mon;
   while(cp) {
     
     if((F_ISSET(ply_ptr, PDINVI) ? 1:!F_ISSET(cp->crt, MINVIS)) &&
!F_ISSET(cp->crt, MHIDDN) 
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

   chance = (20- ply_ptr->thaco) + 2*bonus[ply_ptr->dexterity] -
enm_thaco / m2 + (ply_ptr->level+29)/30 ;
   chance = MIN(chance, 20);
   if(chance < 5) chance = 5;

   if (mrand(1,22) <= chance) {

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
	print(fd, "최루탄이란 나의 모든기로 적의 mp를 소모 하는것이다.\n");
     	print(fd, "받아라 나의 작고 매운 최루탄을 .... \n");
     k = MIN((chance+1)/3, m2);
     cp = rom_ptr->first_mon;
     for( j=0 ;j<k;j++)  {
       if((F_ISSET(ply_ptr, PDINVI) ? 1:!F_ISSET(cp->crt, MINVIS)) &&
!F_ISSET(cp->crt, MHIDDN) \
	  && !F_ISSET(cp->crt, MUNKIL )) {
	 dmg = (mdice(ply_ptr)*2 +
mdice(ply_ptr->ready[WIELD-1])*4)/mrand(1,5)*mrand(1,2);
	 dmg = MIN(cp->crt->hpcur, dmg);
	 
	 if(ply_ptr->ready[WIELD-1]) {
	   p = MIN(ply_ptr->ready[WIELD-1]->type, MISSILE);
	   addprof = (dmg * cp->crt->experience) / (cp->crt->hpmax * k);
	   addprof = MIN(addprof, cp->crt->experience);
	   ply_ptr->proficiency[p] += addprof;
	 }

	 if(mrand(0,1) && (ply_ptr->piety < cp->crt->piety) &&
F_ISSET(cp->crt, MMGONL)) {
	   print(fd, "당신의 화살이 %M에게 아무런 상처도 내지 못합니다.\n", cp->crt);
	   dmg = 1;
	 }
	 if(F_ISSET(cp->crt, MENONL) &&
ply_ptr->ready[WIELD-1]->adjustment < 1) {
	   print(fd, "당신의 화살은 %M의 갑옷을 뚫기엔 역부족입니다.\n",
cp->crt);
	   dmg = 1;
	 }
	 add_enm_dmg(ply_ptr->name, cp->crt , dmg);
	 dur = MIN(20, dmg/20);
	 
 		 print(fd, "\n당신은 ");
		 ANSI(fd, YELLOW);
		 print(fd, "최루탄");
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "로 %m에게 " , cp->crt);
		 ANSI(fd, GREEN);
		 print(fd, "%d", dmg);
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "의 피해를 입힙니다.\n");

/*	 print(fd, "\n당신은 최루탄으로 %m에게 %d의 피해를 입힙니다.\n",
cp->crt, dmg);     */
	 broadcast_rom(fd, ply_ptr->rom_num,
		       "\n%M이 최루탄으로 %m에게 %d의 피해를 입혔습니다.\n", \
		       ply_ptr,cp->crt , dmg);
	 if(dur > 5) {
	 }
	 cp->crt->hpcur -= dmg;
       }	 
       if(cp->crt->hpcur < 1) {
	 print(fd, "\n당신의 매서운 최루탄으로 %M%j 죽였습니다.", cp->crt
,"3");
	 broadcast_rom(fd, ply_ptr->rom_num,
		       "\n%M%j 최루탄으로 %M%j 죽였습니다.", ply_ptr,"1",
cp->crt,"3");
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
	 print(fd, "최루탄");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이 적의 기세에 눌려 실패했습니다.\n");

     broadcast_rom(fd, ply_ptr->rom_num,
		   "\n%M의 최루탄이 적의 기세에 눌려 실패했습니다.\n",
ply_ptr);
     
     if( !F_ISSET(ply_ptr->ready[WIELD-1], ONSHAT) && \
	 ply_ptr->ready[WIELD-1]->shotscur <
ply_ptr->ready[WIELD-1]->shotsmax/2 && ((mrand(1,5) <= 2))) {
       print(fd, "\n%S%j 산산히 부서집니다.",
	     ply_ptr->ready[WIELD-1]->name,"1");
       broadcast_rom(fd, ply_ptr->rom_num,
		     "\n%s의 %S%j 산산히 부서집니다.",
		     F_ISSET(ply_ptr, PMALES) ? "그":"그녀",
		     ply_ptr->ready[WIELD-1]->name,"1");
       free_obj(ply_ptr->ready[WIELD-1]);
       ply_ptr->ready[WIELD-1] = 0;
     }
     
   }
   ply_choi_time[fd] = t+ (18 -MIN(7, ply_ptr->dexterity/4));
   return(0);
}
/**********************************************************************/
/*                      remove blindness(실명해소술)               */
/**********************************************************************/
 
int rm_blind2(ply_ptr, cmnd)
creature    *ply_ptr;
cmd     *cmnd;
{
    room        *rom_ptr;
    creature    *crt_ptr;
    int     fd;
 	int chance;

    fd = ply_ptr->fd;
    rom_ptr = ply_ptr->parent_rom;
 
	if(ply_ptr->mpcur < 20) {
        print(fd, "당신의 도력이 부족합니다");
        return(0);
    }
 	if(!S_ISSET(ply_ptr, YELLOWI)) {
 		print(fd, "아직 당신에게 그런능력이 없습니다.");
		return(0);
	}
 	if(!F_ISSET(ply_ptr, PBLIND)) {
		print(fd, "실명이 되었을때만 사용할수 있습니다.");
		return(0);
	}	
    if(cmnd->num >= 2) {
 		print(fd, "실명해소술은 자신 치료 기술입니다.");
		return(0);
	}
		F_CLR(ply_ptr, PBLIND);
		print(fd, "\n당신은 손에서 개안부를 만들어 눈을찾습니다.\n");
		print(fd, "그 개안부를 눈에 붙이니 당신의 눈이 다시 떠집니다. \n");
		ply_ptr->mpcur -= 20;
	    broadcast_rom(fd, ply_ptr->rom_num, 
                      "\n%M이 손에서 개안부를 만들어 눈에 붙입니다. \n
						감겼던 그의 눈이 다시 떠집니다.", ply_ptr);


    return(0);
 
}

